package collect

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/juanpabloaj/workpulse/internal/model"
)

type ProcessCollector struct{}

func NewProcessCollector() *ProcessCollector {
	return &ProcessCollector{}
}

func (c *ProcessCollector) Collect(ctx context.Context) ([]model.ProcessInfo, error) {
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
			CWD:     processCWD(ctx, pid),
		}
		processes = append(processes, process)
	}

	return processes, nil
}

func processCWD(ctx context.Context, pid int) string {
	cmd := exec.CommandContext(ctx, "lsof", "-a", "-p", fmt.Sprintf("%d", pid), "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return strings.TrimPrefix(line, "n")
		}
	}
	return ""
}
