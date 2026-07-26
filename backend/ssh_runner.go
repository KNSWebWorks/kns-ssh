package main

import (
	"bytes"
	"fmt"

	"golang.org/x/crypto/ssh"
)

type SSHResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Error    string
}

func ExecuteSSHCommand(host string, port int, user, authType, password, privateKey, command string) (*SSHResult, error) {
	var authMethods []ssh.AuthMethod

	if authType == "key" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	} else {
		// default to password
		authMethods = append(authMethods, ssh.Password(password))
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %v", err)
	}
	defer session.Close()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(command)
	
	exitCode := 0
	errMsg := ""
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			errMsg = err.Error()
			exitCode = -1
		}
	}

	return &SSHResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
		Error:    errMsg,
	}, nil
}
