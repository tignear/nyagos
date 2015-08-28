package interpreter

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
)

type CommandNotFound struct {
	Name string
	Err  error
}

func (this CommandNotFound) Stringer() string {
	return fmt.Sprintf("'%s' is not recognized as an internal or external command,\noperable program or batch file", this.Name)
}

func (this CommandNotFound) Error() string {
	return this.Stringer()
}

type ErrorLevel int

const (
	CONTINUE ErrorLevel = 0
	THROUGH  ErrorLevel = -1
	SHUTDOWN ErrorLevel = -2
)

func (this ErrorLevel) HasValue() bool {
	return this >= CONTINUE
}

func (this ErrorLevel) HasError() bool {
	return this > CONTINUE
}

func (this ErrorLevel) String() string {
	switch this {
	case THROUGH:
		return "THROUGH"
	case SHUTDOWN:
		return "SHUTDOWN"
	default:
		return fmt.Sprintf("%d", this)
	}
}

type Interpreter struct {
	exec.Cmd
	Stdio        [3]*os.File
	HookCount    int
	Closer       []io.Closer
	Tag          interface{}
	PipeSeq      [2]uint
	IsBackGround bool
}

func (this *Interpreter) closeAtEnd() {
	if this.Closer != nil {
		for _, c := range this.Closer {
			c.Close()
		}
		this.Closer = nil
	}
}

func New() *Interpreter {
	this := Interpreter{
		Stdio: [3]*os.File{os.Stdin, os.Stdout, os.Stderr},
	}
	this.Stdin = os.Stdin
	this.Stdout = os.Stdout
	this.Stderr = os.Stderr
	this.PipeSeq[0] = pipeSeq
	this.PipeSeq[1] = 0
	this.Tag = nil
	return &this
}

func (this *Interpreter) SetStdin(f *os.File) {
	this.Stdio[0] = f
	this.Stdin = f
}
func (this *Interpreter) SetStdout(f *os.File) {
	this.Stdio[1] = f
	this.Stdout = f
}
func (this *Interpreter) SetStderr(f *os.File) {
	this.Stdio[2] = f
	this.Stderr = f
}

func (this *Interpreter) Clone() *Interpreter {
	rv := new(Interpreter)
	rv.Stdio[0] = this.Stdio[0]
	rv.Stdio[1] = this.Stdio[1]
	rv.Stdio[2] = this.Stdio[2]
	rv.Stdin = this.Stdin
	rv.Stdout = this.Stdout
	rv.Stderr = this.Stderr
	rv.HookCount = this.HookCount
	rv.Tag = this.Tag
	rv.PipeSeq = rv.PipeSeq
	rv.Closer = nil
	return rv
}

type ArgsHookT func(it *Interpreter, args []string) []string

var argsHook = func(it *Interpreter, args []string) []string {
	return args
}

func SetArgsHook(argsHook_ ArgsHookT) (rv ArgsHookT) {
	rv, argsHook = argsHook, argsHook_
	return
}

type HookT func(*Interpreter) (ErrorLevel, error)

var hook = func(*Interpreter) (ErrorLevel, error) {
	return THROUGH, nil
}

func SetHook(hook_ HookT) (rv HookT) {
	rv, hook = hook, hook_
	return
}

var OnCommandNotFound = func(this *Interpreter, err error) error {
	err = &CommandNotFound{this.Args[0], err}
	fmt.Fprintln(this.Stderr, err.Error())
	return err
}

var errorStatusPattern = regexp.MustCompile("^exit status ([0-9]+)")
var ErrorLevelStr string

func nvl(a *os.File, b *os.File) *os.File {
	if a != nil {
		return a
	} else {
		return b
	}
}

func (this *Interpreter) Spawnvp() (ErrorLevel, error) {
	var errorlevel ErrorLevel = CONTINUE
	var err error = nil

	if len(this.Args) > 0 {
		errorlevel, err = hook(this)
		if errorlevel == THROUGH {
			this.Path, err = exec.LookPath(this.Args[0])
			if err == nil {
				err = this.Run()
			} else {
				err = OnCommandNotFound(this, err)
			}
			errorlevel = CONTINUE
		}
	}
	if err != nil {
		fmt.Fprintln(this.Stderr, err.Error())
	}
	return errorlevel, err
}

type result_t struct {
	NextValue ErrorLevel
	Error     error
}

var pipeSeq uint = 0

func (this *Interpreter) Interpret(text string) (errorlevel ErrorLevel, err error) {
	errorlevel = CONTINUE
	err = nil

	statements, statementsErr := Parse(text)
	if statementsErr != nil {
		return CONTINUE, statementsErr
	}
	if argsHook != nil {
		for _, pipeline := range statements {
			for _, state := range pipeline {
				state.Argv = argsHook(this, state.Argv)
			}
		}
	}
	for _, pipeline := range statements {
		var pipeIn *os.File = nil
		pipeSeq++
		isBackGround := false
		for _, state := range pipeline {
			if state.Term == "&" {
				isBackGround = true
				break
			}
		}

		for i, state := range pipeline {
			cmd := new(Interpreter)
			cmd.PipeSeq[0] = pipeSeq
			cmd.PipeSeq[1] = uint(1 + i)
			cmd.IsBackGround = isBackGround
			cmd.Tag = this.Tag
			cmd.HookCount = this.HookCount
			cmd.SetStdin(nvl(this.Stdio[0], os.Stdin))
			cmd.SetStdout(nvl(this.Stdio[1], os.Stdout))
			cmd.SetStderr(nvl(this.Stdio[2], os.Stderr))

			var err error = nil

			if pipeIn != nil {
				cmd.SetStdin(pipeIn)
				cmd.Closer = append(cmd.Closer, pipeIn)
				pipeIn = nil
			}

			if state.Term[0] == '|' {
				var pipeOut *os.File
				pipeIn, pipeOut, err = os.Pipe()
				cmd.SetStdout(pipeOut)
				if state.Term == "|&" {
					cmd.SetStderr(pipeOut)
				}
				cmd.Closer = append(cmd.Closer, pipeOut)
			}

			for _, red := range state.Redirect {
				err = red.OpenOn(cmd)
				if err != nil {
					return CONTINUE, err
				}
			}

			cmd.Args = state.Argv
			if i == len(pipeline)-1 && state.Term != "&" {
				errorlevel, err = cmd.Spawnvp()
				cmd.closeAtEnd()
				if err != nil {
					m := errorStatusPattern.FindStringSubmatch(err.Error())
					if m != nil {
						ErrorLevelStr = m[1]
						err = nil
					} else if errorlevel.HasError() {
						ErrorLevelStr = errorlevel.String()
					} else {
						ErrorLevelStr = "-1"
					}
				} else {
					ErrorLevelStr = errorlevel.String()
				}
			} else {
				go func(cmd1 *Interpreter) {
					cmd1.Spawnvp()
					cmd1.closeAtEnd()
				}(cmd)
			}
		}
	}
	return
}
