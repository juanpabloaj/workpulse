package collect

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

type ProcessCollector struct {
	cache    []model.ProcessInfo
	cachedAt time.Time
	cacheTTL time.Duration
}

func NewProcessCollector() *ProcessCollector {
	return &ProcessCollector{
		cacheTTL: 5 * time.Second,
	}
}

func (c *ProcessCollector) Collect(ctx context.Context) ([]model.ProcessInfo, error) {
	if len(c.cache) > 0 && time.Since(c.cachedAt) < c.cacheTTL {
		return append([]model.ProcessInfo(nil), c.cache...), nil
	}

	cmd := exec.CommandContext(
		ctx,
		"ps",
		"-axo",
		"pid=,ppid=,pcpu=,rss=,etime=,state=,tty=,comm=,args=",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var processes []model.ProcessInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		command := fields[7]
		args := strings.Join(fields[8:], " ")
		agentHint := strings.ToLower(command + " " + args)
		if !containsAnyLower(agentHint, "claude", "codex", "gemini") {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		rssKB, _ := strconv.Atoi(fields[3])

		process := model.ProcessInfo{
			PID:     pid,
			PPID:    ppid,
			CPU:     cpu,
			RSSMB:   rssKB / 1024,
			Elapsed: fields[4],
			Status:  fields[5],
			TTY:     fields[6],
			Command: command,
			Args:    args,
		}
		processes = append(processes, process)
	}

	cwds := batchCWD(ctx, processes)
	for i := range processes {
		processes[i].CWD = cwds[processes[i].PID]
	}

	c.cache = append([]model.ProcessInfo(nil), processes...)
	c.cachedAt = time.Now()
	return append([]model.ProcessInfo(nil), processes...), nil
}

func batchCWD(ctx context.Context, processes []model.ProcessInfo) map[int]string {
	if len(processes) == 0 {
		return nil
	}

	pids := make([]string, 0, len(processes))
	for _, process := range processes {
		if process.PID > 0 {
			pids = append(pids, fmt.Sprintf("%d", process.PID))
		}
	}
	if len(pids) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, "lsof", "-Fn", "-d", "cwd", "-p", strings.Join(pids, ","))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return nil
		}
	}
	return parseBatchCWDOutput(stdout.String())
}

func parseBatchCWDOutput(out string) map[int]string {
	cwds := make(map[int]string)
	currentPID := 0
	currentFD := ""

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "p"):
			currentPID, _ = strconv.Atoi(strings.TrimPrefix(line, "p"))
			currentFD = ""
		case strings.HasPrefix(line, "f"):
			currentFD = strings.TrimPrefix(line, "f")
		case strings.HasPrefix(line, "n") && currentPID != 0 && currentFD == "cwd":
			cwds[currentPID] = strings.TrimPrefix(line, "n")
		}
	}

	return cwds
}
