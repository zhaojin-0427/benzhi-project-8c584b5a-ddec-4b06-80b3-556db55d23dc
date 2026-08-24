package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	defaultAddress = "127.0.0.1:19081"
	defaultSecret  = "local-conservation-credential-key-v1"
)

type config struct {
	address   string
	dataDir   string
	secret    string
	selfcheck bool
}

func parseConfig(args []string) (config, error) {
	defaultAddr, err := addressFromEnvironment(os.Getenv("PORT"))
	if err != nil {
		return config{}, err
	}
	flags := flag.NewFlagSet("conservation", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	address := flags.String("addr", defaultAddr, "HTTP 监听地址（仅回环地址）")
	dataDir := flags.String("data", ".data/conservation", "事件日志和投影目录")
	secret := flags.String("credential-secret", defaultSecret, "本地凭据签发密钥")
	selfcheck := flags.Bool("selfcheck", false, "启动真实 HTTP 服务并执行有界业务冒烟")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}
	if err := validateAddress(*address); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*dataDir) == "" {
		return config{}, fmt.Errorf("-data 不能为空")
	}
	if len(strings.TrimSpace(*secret)) < 16 {
		return config{}, fmt.Errorf("-credential-secret 至少需要 16 个字符")
	}
	return config{address: *address, dataDir: *dataDir, secret: *secret, selfcheck: *selfcheck}, nil
}

func addressFromEnvironment(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return defaultAddress, nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("PORT 必须是 1 至 65535 的端口号")
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(number)), nil
}

func validateAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("-addr 必须是 host:port: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("-addr 仅允许明确的回环 IP 地址")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("-addr 端口必须在 1 至 65535 之间")
	}
	return nil
}
