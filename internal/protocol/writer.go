package protocol

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

const writeBufSize = 4 << 10

const (
	typeSimple = '+'
	typeError  = '-'
	typeInt    = ':'
	typeBulk   = '$'
	typeArray  = '*'
)

const crlf = "\r\n"

func NewWriter(conn io.Writer) *bufio.Writer {
	return bufio.NewWriterSize(conn, writeBufSize)
}

func WriteSimple(w *bufio.Writer, s string) error {
	if err := w.WriteByte(typeSimple); err != nil {
		return err
	}

	if _, err := w.WriteString(sanitizeLine(s)); err != nil {
		return err
	}

	if _, err := w.WriteString(crlf); err != nil {
		return err
	}

	return nil
}

func WriteError(w *bufio.Writer, msg string) error {
	if err := w.WriteByte(typeError); err != nil {
		return err
	}

	if _, err := w.WriteString(sanitizeLine(msg)); err != nil {
		return err
	}

	if _, err := w.WriteString(crlf); err != nil {
		return err
	}

	return nil
}

func WriteInt(w *bufio.Writer, n int) error {
	if err := w.WriteByte(typeInt); err != nil {
		return err
	}

	if _, err := w.WriteString(strconv.Itoa(n)); err != nil {
		return err
	}

	if _, err := w.WriteString(crlf); err != nil {
		return err
	}

	return nil
}

func WriteBulk(w *bufio.Writer, b []byte) error {
	if err := writeLen(w, typeBulk, len(b)); err != nil {
		return err
	}

	if _, err := w.Write(b); err != nil {
		return err
	}

	if _, err := w.WriteString(crlf); err != nil {
		return err
	}

	return nil
}

func WriteNull(w *bufio.Writer) error {
	if _, err := w.WriteString("$-1" + crlf); err != nil {
		return err
	}

	return nil
}

func WriteArrayHeader(w *bufio.Writer, n int) error {
	return writeLen(w, typeArray, n)
}

func WriteStringArray(w *bufio.Writer, strs []string) error {
	if err := WriteArrayHeader(w, len(strs)); err != nil {
		return err
	}

	for _, str := range strs {
		if err := WriteBulk(w, []byte(str)); err != nil {
			return err
		}
	}

	return nil
}

func writeLen(w *bufio.Writer, typ byte, n int) error {
	if err := w.WriteByte(typ); err != nil {
		return err
	}

	if _, err := w.WriteString(strconv.Itoa(n)); err != nil {
		return err
	}

	if _, err := w.WriteString(crlf); err != nil {
		return err
	}

	return nil
}

func sanitizeLine(str string) string {
	if !strings.ContainsAny(str, "\r\n") {
		return str
	}

	return strings.NewReplacer("\r", " ", "\n", " ").Replace(str)
}
