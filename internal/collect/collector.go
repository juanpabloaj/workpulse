package collect

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

type Collector struct {
	claude *ClaudeCollector
	codex  *CodexCollector
	gemini *GeminiCollector
	proc   *ProcessCollector
}

func NewCollector() *Collector {
	return &Collector{
		claude: NewClaudeCollector(),
		codex:  NewCodexCollector(),
		gemini: NewGeminiCollector(),
		proc:   NewProcessCollector(),
	}
}

func (c *Collector) Collect(ctx context.Context) (model.Snapshot, error) {
	processes, err := c.proc.Collect(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}

	procByPID := make(map[int]model.ProcessInfo, len(processes))
	procByCWD := make(map[string][]model.ProcessInfo)
	for _, proc := range processes {
		procByPID[proc.PID] = proc
		if proc.CWD != "" {
			procByCWD[proc.CWD] = append(procByCWD[proc.CWD], proc)
		}
	}

	var sessions []model.Session

	claudeSessions, err := c.claude.Collect(ctx)
	if err == nil {
		sessions = append(sessions, correlateSessions(claudeSessions, procByPID, procByCWD)...)
	}

	codexSessions, err := c.codex.Collect(ctx)
	if err == nil {
		sessions = append(sessions, correlateSessions(codexSessions, procByPID, procByCWD)...)
	}

	geminiSessions, err := c.gemini.Collect(ctx)
	if err == nil {
		sessions = append(sessions, correlateSessions(geminiSessions, procByPID, procByCWD)...)
	}

	sessions = dedupeLiveSessions(sessions)

	sort.Slice(sessions, func(i, j int) bool {
		if stateRank(sessions[i].State) != stateRank(sessions[j].State) {
			return stateRank(sessions[i].State) < stateRank(sessions[j].State)
		}
		if isLiveSession(sessions[i]) && !isLiveSession(sessions[j]) {
			return true
		}
		if !isLiveSession(sessions[i]) && isLiveSession(sessions[j]) {
			return false
		}
		if isRecentSession(sessions[i]) && !isRecentSession(sessions[j]) {
			return true
		}
		if !isRecentSession(sessions[i]) && isRecentSession(sessions[j]) {
			return false
		}
		return sessions[i].LastEventAt.After(sessions[j].LastEventAt)
	})

	return model.Snapshot{
		CollectedAt: time.Now(),
		Sessions:    sessions,
		Processes:   processes,
	}, nil
}

func correlateSessions(
	sessions []model.Session,
	procByPID map[int]model.ProcessInfo,
	procByCWD map[string][]model.ProcessInfo,
) []model.Session {
	for i := range sessions {
		session := &sessions[i]

		if session.Process != nil {
			if proc, ok := procByPID[session.Process.PID]; ok {
				p := proc
				session.Process = &p
			}
		}

		if session.Process == nil && session.CWD != "" {
			candidates := procByCWD[session.CWD]
			var matched []model.ProcessInfo
			for _, candidate := range candidates {
				if processMatchesAgent(candidate, session.Agent) {
					matched = append(matched, candidate)
				}
			}
			if len(matched) > 0 {
				best := bestProcessMatch(session.Agent, matched)
				p := best
				session.Process = &p
			}
		}

		session.State = deriveState(*session)
	}
	return sessions
}

func dedupeLiveSessions(sessions []model.Session) []model.Session {
	bestByKey := map[string]model.Session{}
	var result []model.Session

	for _, session := range sessions {
		if session.Process == nil || session.Process.PID == 0 {
			result = append(result, session)
			continue
		}

		key := string(session.Agent) + ":" + session.CWD + ":" + strconv.Itoa(session.Process.PID)
		existing, ok := bestByKey[key]
		if !ok {
			bestByKey[key] = session
			continue
		}
		if preferSession(session, existing) {
			bestByKey[key] = session
		}
	}

	for _, session := range bestByKey {
		result = append(result, session)
	}
	return result
}

func preferSession(candidate, current model.Session) bool {
	candidateScore := sessionPreferenceScore(candidate)
	currentScore := sessionPreferenceScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate.LastEventAt.After(current.LastEventAt)
}

func sessionPreferenceScore(session model.Session) int {
	score := 0
	switch session.Origin {
	case model.OriginMerged:
		score += 30
	case model.OriginLiveIndex:
		score += 20
	case model.OriginTranscript:
		score += 10
	}
	if session.Model != "" {
		score += 5
	}
	if session.Branch != "" {
		score += 5
	}
	if session.Tools.ToolCalls > 0 {
		score += 5
	}
	return score
}

func deriveState(session model.Session) model.SessionState {
	if session.Tools.PermissionDeny > 0 && containsAnyLower(session.LastEvent, "denied", "approval", "permission") {
		return model.StateBlocked
	}
	if session.Tools.ToolErrors > 0 && containsAnyLower(session.LastEvent, "error", "failed", "denied") {
		return model.StateError
	}
	if session.Process == nil {
		return model.StateDone
	}
	if session.Process.CPU > 2.0 {
		return model.StateRunning
	}

	age := time.Since(session.LastEventAt)
	if age <= 15*time.Second {
		return model.StateRunning
	}
	if age > 15*time.Second {
		return model.StateIdle
	}
	return model.StateIdle
}

func isLiveSession(session model.Session) bool {
	return session.Process != nil
}

func isRecentSession(session model.Session) bool {
	if session.LastEventAt.IsZero() {
		return false
	}
	return time.Since(session.LastEventAt) <= 24*time.Hour
}

func stateRank(state model.SessionState) int {
	switch state {
	case model.StateBlocked:
		return 0
	case model.StateError:
		return 1
	case model.StateRunning:
		return 2
	case model.StateIdle:
		return 3
	case model.StateDone:
		return 4
	default:
		return 5
	}
}

func bestProcessMatch(agent model.AgentKind, candidates []model.ProcessInfo) model.ProcessInfo {
	best := candidates[0]
	bestScore := processMatchScore(agent, best)
	for _, candidate := range candidates[1:] {
		score := processMatchScore(agent, candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func processMatchScore(agent model.AgentKind, proc model.ProcessInfo) int {
	text := strings.ToLower(proc.Command + " " + proc.Args)
	score := 0

	switch agent {
	case model.AgentCodex:
		if strings.Contains(text, "/codex/codex") || strings.HasSuffix(strings.ToLower(proc.Command), "/codex") || strings.Contains(text, " codex") {
			score += 40
		}
		if strings.Contains(text, "node") {
			score -= 10
		}
	case model.AgentClaude:
		if strings.Contains(text, " claude") || strings.HasSuffix(strings.ToLower(proc.Command), "claude") {
			score += 40
		}
	case model.AgentGemini:
		if strings.Contains(text, " gemini") || strings.HasSuffix(strings.ToLower(proc.Command), "gemini") {
			score += 40
		}
	}

	if proc.CPU > 0 {
		score += 5
	}
	if proc.PID > 0 {
		score += 1
	}
	return score
}
