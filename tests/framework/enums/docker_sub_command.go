/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package enums

import (
	"fmt"
	"strings"
)

//DockerSubCommand is enum of docker commands
type DockerSubCommand int

const (
	//Exec docker command
	Exec DockerSubCommand = iota + 1
	//Run docker command
	Run
	//Copy docker command
	Copy
	//Ps docker command
	Ps
	//Pull docker command
	Pull
	//Save docker command
	Save
	//Kill docker command
	Kill
	//Rm docker command
	Rm
	//Empty docker command
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

//GetDockerSubCommandFromString returns docker sub command
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
