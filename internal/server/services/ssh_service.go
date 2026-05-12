package services

import (
	"fmt"

	"golang.org/x/crypto/ssh"
)

// SSHClient wraps a single SSH connection for remote execution.
type SSHClient struct {
	host     string
	port     int
	user     string
	password string
	key      []byte
}

func NewSSHClient(host string, port int, user, password string, privateKey []byte) *SSHClient {
	return &SSHClient{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		key:      privateKey,
	}
}

// Exec runs a command on the remote host.
func (c *SSHClient) Exec(cmd string) (string, error) {
	cfg := &ssh.ClientConfig{
		User:            c.user,
		Auth:            []ssh.AuthMethod{ssh.Password(c.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port), cfg)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// TestConnection checks if SSH is reachable.
func (c *SSHClient) TestConnection() (string, error) {
	return c.Exec("echo ok && uname -a")
}

// DeployAgent installs the agent and systemd service on the remote host.
func (c *SSHClient) DeployAgent(serverAddr, token, name string, backbone bool) (string, error) {
	bf := ""
	if !backbone {
		bf = " \\\n  --backbone=false"
	}

	script := fmt.Sprintf(
		"systemctl stop mesnet-agent 2>/dev/null; "+
			"curl -fsSL https://meshnet.kisectool.com/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && "+
			"chmod +x /usr/local/bin/mesnet-agent && "+
			"printf '[Unit]\\nDescription=MeshNet Agent\\nAfter=network-online.target\\n[Service]\\nType=simple\\nExecStart=/usr/local/bin/mesnet-agent --server ws://%%s/ws/agent/%%s --listen :443 --name \"%%s\"%%s\\nRestart=always\\n[Install]\\nWantedBy=multi-user.target\\n' > /etc/systemd/system/mesnet-agent.service && "+
			"systemctl daemon-reload && systemctl enable --now mesnet-agent",
		serverAddr, token, name, bf)

	return c.Exec(script)
}
