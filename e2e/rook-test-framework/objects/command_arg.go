package objects


type Command_Args struct {
	Command             string
	SubCommand		string
	CmdArgs             []string
	OptionalArgs        []string
	PipeToStdIn         string
	EnvironmentVariable []string
}



type Command_Out struct {
	StdOut string
	StdErr string
	ExitCode int
	Err error
}
