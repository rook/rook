package enums

import (
	"fmt"
	"strings"
)

type DockerSubCommand int

const (
	Exec DockerSubCommand = iota + 1
	Run
	Copy
	Ps
	Pull
	Save
	Kill
	Rm
	Empty
)

var dockerSubCommands = [...]string{
	"exec",
	"run",
	"cp",
	"ps",
	"pull",
	"save",
	"kill",
	"rm",
	"Empty",
}

func (subCommand DockerSubCommand) String() string {
	return dockerSubCommands[subCommand-1]
}

func GetDockerSubCommandFromString(name string) (DockerSubCommand, error) {
	switch {
	case strings.EqualFold(name, Exec.String()):
		return Exec, nil
	case strings.EqualFold(name, Run.String()):
		return Run, nil
	case strings.EqualFold(name, Copy.String()):
		return Copy, nil
	case strings.EqualFold(name, Ps.String()):
		return Ps, nil
	case strings.EqualFold(name, Pull.String()):
		return Pull, nil
	case strings.EqualFold(name, Save.String()):
		return Save, nil
	case strings.EqualFold(name, Kill.String()):
		return Kill, nil
	case strings.EqualFold(name, Rm.String()):
		return Rm, nil
	case strings.EqualFold(name, Empty.String()):
		return Empty, nil
	default:
		return Empty, fmt.Errorf("Unsupported Docker sub-command")
	}
}
