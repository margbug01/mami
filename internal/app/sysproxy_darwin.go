package app

import (
	"net/url"
	"os/exec"
)

func platformSystemProxy() *url.URL {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return nil
	}
	return parseScutilProxy(string(out))
}
