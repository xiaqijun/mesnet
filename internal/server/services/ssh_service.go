package services

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHClient struct {
	host     string
	port     int
	user     string
	password string
	key      []byte
}

func NewSSHClient(host string, port int, user, password string, privateKey []byte) *SSHClient {
	return &SSHClient{host: host, port: port, user: user, password: password, key: privateKey}
}

func (c *SSHClient) connect() (*ssh.Client, error) {
	var auth []ssh.AuthMethod
	if len(c.key) > 0 {
		signer, err := ssh.ParsePrivateKey(c.key)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else {
		auth = append(auth, ssh.Password(c.password))
	}

	cfg := &ssh.ClientConfig{
		User:            c.user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	return ssh.Dial("tcp", fmt.Sprintf("%s:%d", c.host, c.port), cfg)
}

func (c *SSHClient) Exec(cmd string) (string, error) {
	client, err := c.connect()
	if err != nil {
		return "", fmt.Errorf("connect: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

func (c *SSHClient) TestConnection() (string, error) {
	return c.Exec("echo ok && uname -a")
}

func (c *SSHClient) DeployAgent(serverAddr, token, name string, backbone bool) (string, error) {
	bf := ""
	if !backbone {
		bf = " --backbone=false"
	}

	cmd := fmt.Sprintf(
		"systemctl stop mesnet-agent 2>/dev/null; "+
			"curl -fsSL https://meshnet.kisectool.com/mesnet-agent-linux-amd64 -o /usr/local/bin/mesnet-agent && "+
			"chmod +x /usr/local/bin/mesnet-agent && "+
			"printf '[Unit]\\nDescription=MeshNet Agent\\nAfter=network-online.target\\n[Service]\\nType=simple\\nExecStart=/usr/local/bin/mesnet-agent --server ws://%s/ws/agent/%s --listen :443 --name \"%s\"%s\\nRestart=always\\n[Install]\\nWantedBy=multi-user.target\\n' > /etc/systemd/system/mesnet-agent.service && "+
			"systemctl daemon-reload && systemctl enable --now mesnet-agent",
		serverAddr, token, name, bf)

	return c.Exec(cmd)
}
