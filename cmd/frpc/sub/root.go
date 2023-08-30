package sub

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/auth"
	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/pkg/util/version"
)

var (
	cfgFile       string
	cfgDir        string
	showVersion   bool
	userToken     string
	tunnelId      string
	RemoteContent string
	_             string

	serverAddr      string
	user            string
	protocol        string
	token           string
	logLevel        string
	logFile         string
	logMaxDays      int
	disableLogColor bool
	dnsServer       string

	proxyName          string
	localIP            string
	localPort          int
	remotePort         int
	useEncryption      bool
	useCompression     bool
	bandwidthLimit     string
	bandwidthLimitMode string
	customDomains      string
	subDomain          string
	httpUser           string
	httpPwd            string
	locations          string
	hostHeaderRewrite  string
	role               string
	sk                 string
	multiplexer        string
	serverName         string
	bindAddr           string
	bindPort           int

	tlsEnable     bool
	tlsServerName string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file of frpc")
	rootCmd.PersistentFlags().StringVarP(&cfgDir, "config_dir", "", "", "config directory, run one frpc service for each file in config directory")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "version of frpc")
	rootCmd.PersistentFlags().StringVarP(&userToken, "token", "t", "", "You User Token")
	rootCmd.PersistentFlags().StringVarP(&tunnelId, "tunnelId", "i", "", "Tunnel's ID")
}

func RegisterCommonFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&serverAddr, "server_addr", "s", "127.0.0.1:7000", "frp server's address")
	cmd.PersistentFlags().StringVarP(&user, "user", "u", "", "user")
	cmd.PersistentFlags().StringVarP(&protocol, "protocol", "p", "tcp", "tcp, kcp, quic, websocket, wss")
	cmd.PersistentFlags().StringVarP(&token, "token", "t", "", "auth token")
	cmd.PersistentFlags().StringVarP(&logLevel, "log_level", "", "info", "log level")
	cmd.PersistentFlags().StringVarP(&logFile, "log_file", "", "console", "console or file path")
	cmd.PersistentFlags().IntVarP(&logMaxDays, "log_max_days", "", 3, "log file reversed days")
	cmd.PersistentFlags().BoolVarP(&disableLogColor, "disable_log_color", "", false, "disable log color in console")
	cmd.PersistentFlags().BoolVarP(&tlsEnable, "tls_enable", "", true, "enable frpc tls")
	cmd.PersistentFlags().StringVarP(&tlsServerName, "tls_server_name", "", "", "specify the custom server name of tls certificate")
	cmd.PersistentFlags().StringVarP(&dnsServer, "dns_server", "", "", "specify dns server instead of using system default one")
}
func EasyStartGetConf(token string, tunnelId string) {
	req, err := http.NewRequest("GET", "https://api.mefrp.com/api/v2/tunnel/conf/id/"+tunnelId, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)

	c := &http.Client{}

	response, err := c.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	if response.StatusCode != http.StatusOK {
		err = fmt.Errorf("ME Frp API 错误 可能是您的启动信息错误%d", response.StatusCode)
		fmt.Println(err)
		os.Exit(1)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(response.Body)

	data, err := io.ReadAll(response.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("获取配置文件成功！ 启动隧道")
	Content := string(data)
	RemoteContent = Content
	return
}

var rootCmd = &cobra.Command{
	Use:   "frpc",
	Short: "The Frp Client of ME Frp",
	RunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			fmt.Println(version.Full())
			return nil
		}

		if cfgFile == "" && cfgDir == "" && (userToken == "" || tunnelId == "") {
			fmt.Println("启动参数不存在或不完整！ 无法正常使用 EasyStartSingle/Multi 以及 LocalConfigSingle/Multi 启动,即将尝试通过 ./frpc.ini 文件启动。")
			cfgFile = "./frpc.ini"
		}

		// 多隧道启动部分

		// If cfgDir is not empty, run multiple frpc service for each config file in cfgDir.
		// Note that it's only designed for testing. It's not guaranteed to be stable.
		if cfgDir != "" {
			// 使用配置文件夹启动
			fmt.Println("使用配置文件夹启动")
			_ = runMultipleClients(cfgDir)
			return nil
		}
		// 如果 tunnelId 后面跟了多个数字，那么就是多个隧道
		if strings.Contains(tunnelId, ",") {
			fmt.Println("检测到多个隧道，正在启动多个隧道")
			_ = runMultipleClientsEasyStart(userToken, tunnelId)
		}

		// 多隧道不行就单隧道
		err := runClient(cfgFile, userToken, tunnelId)
		if err != nil {
			os.Exit(1)
		}
		return nil
	},
}

func runMultipleClients(cfgDir string) error {
	var wg sync.WaitGroup
	err := filepath.WalkDir(cfgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		wg.Add(1)
		time.Sleep(time.Millisecond)
		go func() {
			defer wg.Done()
			err := runClient(path, "", "")
			if err != nil {
				fmt.Printf("frpc service error for config file [%s]\n", path)
			}
		}()
		return nil
	})
	wg.Wait()
	return err
}

func runMultipleClientsEasyStart(userToken string, tunnelId string) error {
	var wg sync.WaitGroup

	// 对于多个隧道，我们需要分割它们
	tunnelIdList := strings.Split(tunnelId, ",")
	for _, tunnelId := range tunnelIdList {
		wg.Add(1)
		time.Sleep(time.Millisecond)
		go func() {
			defer wg.Done()
			err := runClient("", userToken, tunnelId)
			if err != nil {
				fmt.Printf("frpc service error for config file [%s]\n", tunnelId)
			}
		}()
		return nil
	}
	wg.Wait()
	return nil
}
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func handleTermSignal(svr *client.Service) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	svr.GracefulClose(500 * time.Millisecond)
}

func parseClientCommonCfgFromCmd() (cfg config.ClientCommonConf, err error) {
	cfg = config.GetDefaultClientConf()

	ipStr, portStr, err := net.SplitHostPort(serverAddr)
	if err != nil {
		err = fmt.Errorf("invalid server_addr: %v", err)
		return
	}

	cfg.ServerAddr = ipStr
	cfg.ServerPort, err = strconv.Atoi(portStr)
	if err != nil {
		err = fmt.Errorf("invalid server_addr: %v", err)
		return
	}

	cfg.User = user
	cfg.Protocol = protocol
	cfg.LogLevel = logLevel
	cfg.LogFile = logFile
	cfg.LogMaxDays = int64(logMaxDays)
	cfg.DisableLogColor = disableLogColor
	cfg.DNSServer = dnsServer

	// Only token authentication is supported in cmd mode
	cfg.ClientConfig = auth.GetDefaultClientConf()
	cfg.Token = token
	cfg.TLSEnable = tlsEnable
	cfg.TLSServerName = tlsServerName

	cfg.Complete()
	if err = cfg.Validate(); err != nil {
		err = fmt.Errorf("parse config error: %v", err)
		return
	}
	return
}

func runClient(cfgFilePath string, userToken string, tunnelId string) error {
	var content string
	if cfgFilePath != "" {
		LocalContent, err := config.GetRenderedConfFromFile(cfgFile)
		if err != nil {
			return err
		}
		content = string(LocalContent)
	} else {
		EasyStartGetConf(userToken, tunnelId)
		content = RemoteContent
	}
	cfg, pxyCfgs, visitorCfgs, err := config.ParseClientConfig(content)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return startService(cfg, pxyCfgs, visitorCfgs, cfgFilePath)
}

func startService(
	cfg config.ClientCommonConf,
	pxyCfgs map[string]config.ProxyConf,
	visitorCfgs map[string]config.VisitorConf,
	cfgFile string,
) (err error) {
	log.InitLog(cfg.LogWay, cfg.LogFile, cfg.LogLevel,
		cfg.LogMaxDays, cfg.DisableLogColor)

	if cfgFile != "" {
		log.Info("start frpc service for config file [%s]", cfgFile)
		defer log.Info("frpc service for config file [%s] stopped", cfgFile)
	}
	svr, errRet := client.NewService(cfg, pxyCfgs, visitorCfgs, cfgFile)
	if errRet != nil {
		err = errRet
		return
	}

	shouldGracefulClose := cfg.Protocol == "kcp" || cfg.Protocol == "quic"
	// Capture the exit signal if we use kcp or quic.
	if shouldGracefulClose {
		go handleTermSignal(svr)
	}

	_ = svr.Run(context.Background())
	return
}
