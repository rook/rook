package enums

import (
	"errors"
	"strings"
)

type DockerSubCommand int

const (
	Exec DockerSubCommand = iota + 1
	Run
	Copy
	Ps
	Empty
)

var dockerSubCommands = [...]string{
	"exec",
	"run",
	"cp",
	"ps",
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
	case strings.EqualFold(name, Empty.String()):
		return Empty, nil
	default:
		return Empty, errors.New("Unsupported Docker sub-command")
	}
}
