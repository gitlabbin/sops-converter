package decrypt

import (
	"bytes"
	"fmt"
	"os/exec"
)

//go:generate moq -out mocks/decryptor_mock.go -pkg decrypt_mocks . Decryptor
type Decryptor interface {
	Decrypt([]byte, string) ([]byte, error)
}

var _ Decryptor = &SopsDecrytor{}

type SopsDecrytor struct {
}

func (d *SopsDecrytor) Decrypt(input []byte, outFormat string) ([]byte, error) {
	args := []string{"--decrypt", "--input-type", outFormat, "--output-type", outFormat, "/dev/stdin"}

	command := exec.Command("sops", args...)
	command.Stdin = bytes.NewBuffer(input)

	output, err := command.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to decrypt file: %s", string(e.Stderr))
		}
		return nil, err
	}
	return output, err
}
