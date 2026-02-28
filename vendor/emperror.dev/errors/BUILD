github_repo(
    name = "pleasings2",
    repo = "sagikazarmark/mypleasings",
    revision = "f8a12721c6f929db3e227e07c152d428ac47ab1b",
)

sh_cmd(
    name = "download_tests",
    shell = "/usr/bin/env bash",
    labels = ["manual"],
    cmd = """
mkdir -p tests
# curl https://raw.githubusercontent.com/pkg/errors/master/errors_test.go | sed \\\\$'s|"testing"|"testing"\\n\\n\\t. "emperror.dev/errors"|; s|github.com/pkg/errors|emperror.dev/errors/tests|g' > tests/errors_test.go
curl https://raw.githubusercontent.com/pkg/errors/master/errors_test.go | sed \\\\$'s|"errors"|. "emperror.dev/errors"|; s|/github.com/pkg/errors||g; s|github.com/pkg/errors|emperror.dev/errors/tests|g; s|errors\.New|NewPlain|g; s|x := New("error")|x := NewPlain("error")|g' > tests/errors_test.go
curl https://raw.githubusercontent.com/pkg/errors/master/example_test.go | sed 's|"github.com/pkg/errors"|"emperror.dev/errors"|' > tests/example_test.go
curl https://raw.githubusercontent.com/pkg/errors/master/format_test.go | sed \\\\$'s|"errors"|. "emperror.dev/errors"|; s|/github.com/pkg/errors||g; s|github.com/pkg/errors|emperror.dev/errors/tests|g; s|errors\.New|NewPlain|g' > tests/format_test.go

curl https://raw.githubusercontent.com/golang/go/master/src/errors/errors_test.go | sed \\\\$'s|"errors"|"emperror.dev/errors"|; s|errors\.New|errors.NewPlain|g; s|"fmt"||g' | head -35 > tests/errors_std_test.go
echo -e "// +build go1.13\n\n\\\$(curl https://raw.githubusercontent.com/golang/go/master/src/errors/wrap_test.go)" | sed \\\\$'s|"errors"|"emperror.dev/errors"|; s|errors\.New|errors.NewPlain|g' > tests/wrap_test.go
""",
)
