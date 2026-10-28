package protocol

import (
	"bufio"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
)

var ErrProtocol = errors.New("protocol error")

const (
	maxBulkLen   = 512 << 20
	bulkChunk    = 64 << 10
	maxArgs      = 1 << 20
	inlineMaxLen = 64<<10 - 2 // 65534 (-2 for CRLF) total (65536)
)

type CmdName string

const (
	PingCmdName   CmdName = "PING"
	SetCmdName    CmdName = "SET"
	GetCmdName    CmdName = "GET"
	ExistsCmdName CmdName = "EXISTS"
	DelCmdName    CmdName = "DEL"
	KeysCmdName   CmdName = "KEYS"
	ScanCmdName   CmdName = "SCAN"
)

type Command struct {
	Name CmdName
	Args [][]byte
}

// create reader with max buf size
func NewReader(conn io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(conn, inlineMaxLen+2)
}

func ReadCommand(r *bufio.Reader) (*Command, error) {
	prefix, err := r.Peek(1)
	if err != nil {
		return nil, err
	}

	if prefix[0] == '*' {
		return readArray(r)
	}

	return readInline(r)
}

func readArray(r *bufio.Reader) (*Command, error) {
	n, err := readCount(r, '*')
	if err != nil {
		return nil, err
	}

	if n <= 0 || n > maxArgs {
		return nil, ErrProtocol
	}

	parts := make([][]byte, n)
	for i := range parts {
		if parts[i], err = readBulk(r); err != nil {
			return nil, err
		}
	}

	if len(parts[0]) == 0 {
		return nil, ErrProtocol
	}

	return &Command{
			Name: CmdName(strings.ToUpper(string(parts[0]))),
			Args: parts[1:]},
		nil
}

func readBulk(r *bufio.Reader) ([]byte, error) {
	n, err := readCount(r, '$')
	if err != nil {
		return nil, err
	}

	if n < 0 || n > maxBulkLen {
		return nil, ErrProtocol
	}

	total := n + 2

	buf := make([]byte, 0, min(total, bulkChunk))
	for len(buf) < total {
		want := min(total-len(buf), bulkChunk)
		start := len(buf)
		buf = slices.Grow(buf, want)[:start+want]

		if _, err := io.ReadFull(r, buf[start:]); err != nil {
			return nil, err
		}
	}

	if buf[n] != '\r' || buf[n+1] != '\n' {
		return nil, ErrProtocol
	}

	return buf[:n], nil
}

func readCount(r *bufio.Reader, typ byte) (int, error) {
	line, err := readLine(r)
	if err != nil {
		return 0, err
	}

	if len(line) == 0 || line[0] != typ {
		return 0, ErrProtocol
	}

	n, err := strconv.Atoi(string(line[1:]))
	if err != nil {
		return 0, ErrProtocol
	}

	return n, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, ErrProtocol
	}

	if err != nil {
		return nil, err
	}

	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, ErrProtocol
	}

	return line[:len(line)-2], nil
}

func readInline(r *bufio.Reader) (*Command, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}

	if len(line) > inlineMaxLen {
		return nil, ErrProtocol
	}

	args, err := splitInline(line)
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		return nil, nil
	}

	return &Command{
		Name: CmdName(strings.ToUpper(string(args[0]))),
		Args: args[1:],
	}, nil
}

func splitInline(line []byte) ([][]byte, error) {
	var args [][]byte
	i := 0

	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}

		if i >= len(line) {
			break
		}

		var arg []byte
		switch q := line[i]; q {
		case '"', '\'':
			i++

			for i < len(line) && line[i] != q {
				if q == '"' && line[i] == '\\' && i+1 < len(line) {
					i++ // skip the backslash; in double quotes it escapes the next byte
				}

				arg = append(arg, line[i])
				i++
			}

			if i >= len(line) {
				return nil, ErrProtocol // unterminated quote
			}

			i++
		default:
			for i < len(line) && line[i] != ' ' && line[i] != '\t' {
				arg = append(arg, line[i])
				i++
			}
		}

		args = append(args, arg)
	}

	return args, nil
}
