package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

type config struct {
	addr             string
	database         string
	selfcheck        bool
	selfcheckTimeout time.Duration
}

func parseConfig() (config, error) {
	defaultAddr := "127.0.0.1:19081"
	if raw := os.Getenv("PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return config{}, errors.New("PORT 必须为 1 到 65535 的端口号")
		}
		defaultAddr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	c := config{}
	flag.StringVar(&c.addr, "addr", defaultAddr, "HTTP 监听地址")
	flag.StringVar(&c.database, "db", "rigging-inspection.db", "SQLite 数据库路径")
	flag.BoolVar(&c.selfcheck, "selfcheck", false, "通过真实 HTTP 监听执行完整闭环后退出")
	flag.DurationVar(&c.selfcheckTimeout, "selfcheck-timeout", 20*time.Second, "selfcheck 总超时")
	flag.Parse()
	host, port, err := net.SplitHostPort(c.addr)
	if err != nil {
		return c, fmt.Errorf("解析 -addr: %w", err)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return c, errors.New("为保护检验数据，监听地址必须是回环地址")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, errors.New("监听端口无效")
	}
	if c.selfcheckTimeout <= 0 {
		return c, errors.New("selfcheck-timeout 必须大于零")
	}
	return c, nil
}
