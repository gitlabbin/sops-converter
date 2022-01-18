package util

import "fmt"
import "os/exec"

func Cmd(cmd string, shell bool) []byte {
	if shell {
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		if err != nil {
			fmt.Println(err.Error())
		}
		return out
	} else {
		out, err := exec.Command(cmd).CombinedOutput()
		if err != nil {
			fmt.Println(err.Error())
		}
		return out
	}
}
