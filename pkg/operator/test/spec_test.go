// Package testlib provides common methods for testing code which applies to many Ceph daemons.
//
// Methods beginning with "TestSpec" can be used to test that Kubernetes resource specs (pods, etc.)
// are configured correctly.
package test

import "testing"

// Test the tests!

// make one expected arg
func oneExp(s ...string) [][]string {
	return [][]string{s}
}

// make actual args
func act(s ...string) []string {
	return s
}

func TestArgumentsMatchExpected(t *testing.T) {
	type args struct {
		expectedArgs [][]string
		actualArgs   []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"-f ; ok", args{oneExp("-h"), act("-h")}, false},
		{"--flag=val ; ok", args{oneExp("--show=all"), act("--show=all")}, false},
		{"--flag val ; ok", args{oneExp("--show", "all"), act("--show", "all")}, false},
		{"-fval notfound", args{oneExp("-Ohere"), act("-ohere")}, true},
		{"--flag val ; no flag", args{oneExp("--debug", "3"), act("--iterations", "3")}, true},
		{"--flag val ; no val", args{oneExp("--debug", "3"), act("--debug", "4")}, true},
		{"--flag val ; out of order", args{oneExp("-d", "3"), act("3", "-d")}, true},
		{"empty expected arg", args{oneExp(""), act("-h")}, true},
		{"empty actual", args{oneExp("-h"), act("")}, true},
		{"extra actuals", args{oneExp("-h"), act("-h", "-v")}, true},
		{"complicated ; ok", args{
			[][]string{{"-h"}, {"-vvv"}, {"-d", "3"}, {"--name=kit"}, {"--name", "sammy"}},
			[]string{"-h", "-vvv", "-d", "3", "--name=kit", "--name", "sammy"}}, false},
		{"complicated ; missing", args{
			[][]string{{"-h"}, {"-vvv"}, {"-d", "3"}, {"--name=kit"}, {"--name", "sammy"}},
			[]string{"-h", "-vvv", "-d", "3", "--name", "sammy"}}, true},
		{"complicated ; extra actuals", args{
			[][]string{{"-h"}, {"-vvv"}, {"-d", "3"}, {"--name=kit"}, {"--name", "sammy"}},
			[]string{"-h", "-vvv", "--i-am=extra", "-d", "3", "--name", "sammy"}}, true},
		{"complicated ; double instance", args{
			[][]string{{"-h"}, {"-vvv"}, {"-d", "3"}, {"--name=kit"}, {"--name", "sammy"}},
			[]string{"-h", "-vvv", "-vvv", "-d", "3", "--name", "sammy"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ArgumentsMatchExpected(tt.args.actualArgs, tt.args.expectedArgs); (err != nil) != tt.wantErr {
				t.Errorf("ArgumentsMatchExpected() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
