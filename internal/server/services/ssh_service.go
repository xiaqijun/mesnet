package services

import (
	"fmt"
	"net"
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
	return &SSHClient{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		key:      privateKey,
	}
}

func (c *SSHClient) connect() (*ssh.Client, error) {
	auth := make([]ssh.AuthMethod, 0)
	if len(c.key) > 0 {
		signer, err := ssh.ParsePrivateKey(c.key)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	} else if c.password != "" {
		auth = append(auth, ssh.Password(c.password))
	} else {
		return nil, fmt.Errorf("no auth method")
	}

	config := &ssh.ClientConfig{
		User:            c.user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(c.host, fmt.Sprintf("%d", c.port))
	return ssh.Dial("tcp", addr, config)
}

func (c *SSHClient) Exec(cmd string) (string, error) {
	client, err := c.connect()
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

func (c *SSHClient) DeployAgent(token, name string, backbone bool) (string, error) {
	steps := make([]string, 0)

	// Step 1: Download agent binary from control plane
	steps = append(steps, "download binary")
	dlCmd := fmt.Sprintf(
		"curl -fsSL http://YOUR_SERVER:8080/api/agent/binary -o /usr/local/bin/mesnet-agent && chmod +x /usr/local/bin/mesnet-agent",
	)
	if out, err := c.Exec(dlCmd); err != nil {
		return joinSteps(steps), fmt.Errorf("download: %w (%s)", err, out)
	}

	// Step 2: Write systemd unit
	steps = append(steps, "write systemd unit")
	backboneFlag := ""
	if !backbone {
		backboneFlag = " \\\n  --backbone=false"
	}

	unit := fmt.Sprintf(`[Unit]
Description=MeshNet Agent
After=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mesnet-agent \
  --server wss://YOUR_SERVER/ws/agent/%s \
  --listen :443 \
  --name "%s"%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target`, token, name, backboneFlag)

	writeUnit := fmt.Sprintf("cat > /etc/systemd/system/mesnet-agent.service <<'EOF'\n%s\nEOF", unit)
	if out, err := c.Exec(writeUnit); err != nil {
		return joinSteps(steps), fmt.Errorf("write unit: %w (%s)", err, out)
	}

	// Step 3: Start service
	steps = append(steps, "start service")
	if out, err := c.Exec("systemctl daemon-reload && systemctl enable mesnet-agent && systemctl start mesnet-agent"); err != nil {
		return joinSteps(steps), fmt.Errorf("start: %w (%s)", err, out)
	}

	steps = append(steps, "done")
	return joinSteps(steps), nil
}

func (c *SSHClient) TestConnection() (string, error) {
	return c.Exec("echo ok && uname -a")
}

func joinSteps(steps []string) string {
	result := ""
	for i, s := range steps {
		if i > 0 {
			result += " → "
		}
		result += s
	}
	return result
}
