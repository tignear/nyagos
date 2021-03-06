package shell

import (
	"errors"
	"os"
)

// NoClobber is the switch to forbide to overwrite the exist file.
var NoClobber = false

type _Redirecter struct {
	path     string
	isAppend bool
	no       int
	dupFrom  int
	force    bool
}

func newRedirecter(no int) *_Redirecter {
	return &_Redirecter{
		path:     "",
		isAppend: false,
		no:       no,
		dupFrom:  -1}
}

func (r *_Redirecter) FileNo() int {
	return r.no
}

func (r *_Redirecter) DupFrom(fileno int) {
	r.dupFrom = fileno
}

func (r *_Redirecter) SetPath(path string) {
	r.path = path
}

func (r *_Redirecter) SetAppend() {
	r.isAppend = true
}

func (r *_Redirecter) open() (*os.File, error) {
	if r.path == "" {
		return nil, errors.New("_Redirecter.open(): path=\"\"")
	}
	if r.no == 0 {
		return os.Open(r.path)
	} else if r.isAppend {
		return os.OpenFile(r.path, os.O_APPEND|os.O_CREATE, 0666)
	} else {
		if NoClobber && !r.force {
			_, err := os.Stat(r.path)
			if err == nil {
				return nil, os.ErrExist
			}
		}
		return os.Create(r.path)
	}
}

func (r *_Redirecter) OpenOn(cmd *Cmd) (*os.File, error) {
	var fd *os.File
	var err error

	switch r.dupFrom {
	case 0:
		fd = cmd.Stdin
	case 1:
		fd = cmd.Stdout
	case 2:
		fd = cmd.Stderr
	default:
		fd, err = r.open()
		if err != nil {
			return nil, err
		}
	}
	switch r.FileNo() {
	case 0:
		cmd.Stdin = fd
	case 1:
		cmd.Stdout = fd
	case 2:
		cmd.Stderr = fd
	default:
		panic("Assertion failed: _Redirecter.OpenAs: r.no not in (0,1,2)")
	}
	return fd, nil
}
