package nodeagent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// executeFixed has no remote command surface. Its callers supply embedded helpers only.
func (d *Docker) executeFixed(ctx context.Context, name, script string, args []string, result any) error {
	command := append([]string{"node", "--no-warnings", "-e", script}, args...)
	var created struct{ ID string }
	if _, err := d.call(ctx, "POST", "/containers/"+name+"/exec", map[string]any{"AttachStdout": true, "AttachStderr": true, "Tty": false, "Cmd": command}, &created); err != nil {
		return errors.New("fixed runtime helper unavailable")
	}
	if created.ID == "" {
		return errors.New("runtime helper identity missing")
	}
	request, err := http.NewRequestWithContext(ctx, "POST", "http://docker/v1.44/exec/"+created.ID+"/start", bytes.NewBufferString(`{"Detach":false,"Tty":false}`))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := d.client.Do(request)
	if err != nil {
		return errors.New("fixed runtime helper unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("fixed runtime helper rejected")
	}
	stdout, err := decodeHelperOutput(io.LimitReader(response.Body, 65537))
	if err != nil {
		return err
	}
	var status struct {
		ExitCode *int
		Running  bool
	}
	if _, err = d.call(ctx, "GET", "/exec/"+created.ID+"/json", nil, &status); err != nil || status.Running || (status.ExitCode == nil || *status.ExitCode != 0) {
		return errors.New("fixed runtime helper failed")
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	decoder.DisallowUnknownFields()
	if decoder.Decode(result) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("fixed runtime helper response invalid")
	}
	return nil
}

func decodeHelperOutput(reader io.Reader) ([]byte, error) {
	var output bytes.Buffer
	total := 0
	for {
		var header [8]byte
		_, err := io.ReadFull(reader, header[:])
		if err == io.EOF {
			return output.Bytes(), nil
		}
		if err != nil || header[0] != 1 || header[1] != 0 || header[2] != 0 || header[3] != 0 {
			return nil, errors.New("fixed runtime helper failed")
		}
		length := binary.BigEndian.Uint32(header[4:])
		total += 8 + int(length)
		if length > 16384 || total > 65536 || output.Len()+int(length) > 16384 {
			return nil, errors.New("fixed runtime helper output limit exceeded")
		}
		if _, err = io.CopyN(&output, reader, int64(length)); err != nil {
			return nil, errors.New("fixed runtime helper output incomplete")
		}
	}
}
