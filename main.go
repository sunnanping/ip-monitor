package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"gopkg.in/yaml.v3"
)

// IPResponse 用于解析IP API响应
type IPResponse struct {
	IP string `json:"ip"`
}

// MailConfig 邮件配置
type MailConfig struct {
	SmtpServer string `yaml:"smtp_server"` // SMTP服务器地址
	SmtpPort   string `yaml:"smtp_port"`   // SMTP服务器端口
	Username   string `yaml:"username"`    // SMTP用户名（邮箱地址）
	Password   string `yaml:"password"`    // SMTP密码（邮箱授权码）
	From       string `yaml:"from"`        // 发件人邮箱地址
	To         string `yaml:"to"`          // 收件人邮箱地址（多个地址用英文逗号分隔）
	SendMode   int    `yaml:"send_mode"`   // 邮件发送模式：1-单个发送，3-群发
	Title      string `yaml:"title"`       // 邮件标题
	SenderName string `yaml:"sender_name"` // 邮件发送者名称
}

// TaskPara 任务参数配置
type TaskPara struct {
	CronExpression string `yaml:"cron_expression"` // cron表达式，定义IP检查频率
}

// Config 用于存储配置信息
type Config struct {
	MailConfig *MailConfig `yaml:"mail-config"` // 邮件参数配置
	IPv4List   []string    `yaml:"ip-v4-list"`  // IPv4地址API列表
	IPv6List   []string    `yaml:"ip-v6-list"`  // IPv6地址API列表
	TaskPara   *TaskPara   `yaml:"task-para"`   // 任务参数配置
}

// RunRecord 运行记录
type RunRecord struct {
	LastRunTime string `json:"last_run_time"` // 上一次运行时间
	IPv4        string `json:"ipv4"`          // IPv4地址
	IPv6        string `json:"ipv6"`          // IPv6地址
	EmailSent   bool   `json:"email_sent"`    // 是否发送了邮件
	EmailResult string `json:"email_result"`  // 邮件发送结果
	RunCount    int    `json:"run_count"`     // 累计运行次数
}

// 全局变量存储缓存的IP地址和运行信息
var (
	cachedIPv4   string     // 缓存的IPv4地址
	cachedIPv6   string     // 缓存的IPv6地址
	appConfig    *Config    // 配置信息
	runRecord    *RunRecord // 运行记录
	programPID   int        // 程序进程ID
	runCount     int        // 累计运行次数
	firstRunTime string     // 第一次运行时间（程序启动时间）
	lastRunTime  string     // 上一次运行时间（每次检查IP地址时更新）
	sendMsg      bool       // 是否发送了邮件
	execPath     string     // 程序路径
	execDir      string     // 程序所在目录（用于确保开机启动时能正确读取配置文件）
)

// loadConfig 加载配置文件
// filePath: 配置文件的完整路径
func loadConfig(filePath string) (*Config, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 检查必需的配置项
	if config.MailConfig == nil {
		return nil, fmt.Errorf("邮件配置(mail-config)不能为空")
	}
	if config.TaskPara == nil {
		return nil, fmt.Errorf("任务参数(task-para)不能为空")
	}

	return &config, nil
}

// loadRunRecord 加载运行记录
func loadRunRecord(filePath string) (*RunRecord, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取运行记录文件失败: %v", err)
	}

	var record RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("解析运行记录文件失败: %v", err)
	}

	return &record, nil
}

// saveRunRecord 保存运行记录
func saveRunRecord(filePath string, record *RunRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化运行记录失败: %v", err)
	}

	err = ioutil.WriteFile(filePath, data, 0644)
	if err != nil {
		return fmt.Errorf("写入运行记录文件失败: %v", err)
	}

	return nil
}

// printLastRunLog 输出上一次运行日志
func printLastRunLog(record *RunRecord) {
	if record == nil {
		fmt.Println("第一次运行该程序，首次发送邮件")
		return
	}

	fmt.Println("\n========== 上一次运行记录 ==========")
	fmt.Printf("运行时间: %s\n", record.LastRunTime)
	fmt.Printf("IPv4地址: %s\n", record.IPv4)
	fmt.Printf("IPv6地址: %s\n", record.IPv6)
	fmt.Printf("是否发送邮件: %v\n", record.EmailSent)
	if record.EmailSent {
		fmt.Printf("邮件发送结果: %s\n", record.EmailResult)
	}
	fmt.Println("===================================\n")
}

// getIPAddress 获取公网IP地址
func getIPAddress() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 按稳定性从高到低排序的5个公网IP查询API
	apis := []struct {
		url    string
		isJSON bool
	}{
		{"https://api.ipify.org?format=json", true}, // 1. 最高稳定性 - 专门为开发者设计的免费IP查询服务
		{"https://ifconfig.me/ip", false},           // 2. 最高稳定性 - 老牌IP查询服务，历史悠久
		{"https://ipecho.net/plain", false},         // 3. 很高稳定性 - 专业的IP查询服务
		{"https://api.ip.sb/ip", false},             // 4. 很高稳定性 - 提供详细的IP信息
		{"https://ipinfo.io/ip", false},             // 5. 高稳定性 - 提供丰富的IP信息
	}

	for _, api := range apis {
		resp, err := client.Get(api.url)
		if err != nil {
			fmt.Printf("尝试API %s失败: %v\n", api.url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("API %s返回错误状态码: %d\n", api.url, resp.StatusCode)
			continue
		}

		if api.isJSON {
			var ipResp IPResponse
			if err := json.NewDecoder(resp.Body).Decode(&ipResp); err != nil {
				fmt.Printf("解析API %s响应失败: %v\n", api.url, err)
				continue
			}
			return ipResp.IP, nil
		} else {
			// 处理纯文本响应
			buf := make([]byte, 100)
			n, err := resp.Body.Read(buf)
			if err != nil {
				fmt.Printf("读取API %s响应失败: %v\n", api.url, err)
				continue
			}
			ip := string(buf[:n])
			// 去除空白字符
			ip = strings.TrimSpace(ip)
			return ip, nil
		}
	}

	return "", fmt.Errorf("所有API都失败了")
}

// isValidIPv4 检查IPv4地址的有效性
func isValidIPv4(ip string) bool {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		num, err := fmt.Sscanf(part, "%d", new(int))
		if err != nil || num != 1 {
			return false
		}

		var value int
		fmt.Sscanf(part, "%d", &value)
		if value < 0 || value > 255 {
			return false
		}
	}

	return true
}

// getIPv4Address 获取公网IPv4地址
func getIPv4Address() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 从配置文件中获取IPv4地址API列表，如果为空则使用默认列表
	apis := appConfig.IPv4List
	if len(apis) == 0 {
		// 使用默认的IPv4地址API列表
		apis = []string{
			"https://ipinfo.io/ip",
			"https://api.ipify.org?format=json&ipv4=true",
			"https://ifconfig.me/ip",
			"https://ipecho.net/plain",
			"https://api.ip.sb/ip",
			"https://checkip.amazonaws.com",
			"https://ident.me",
			"https://bot.whatismyipaddress.com",
			"https://myexternalip.com/raw",
			"https://ipaddr.site",
		}
		fmt.Println("使用默认IPv4地址API列表")
	}

	fmt.Println("开始遍历IPv4地址API清单...")

	for i, apiURL := range apis {
		fmt.Printf("正在尝试第%d个IPv4 API: %s\n", i+1, apiURL)

		resp, err := client.Get(apiURL)
		if err != nil {
			fmt.Printf("尝试IPv4 API %s失败: %v\n", apiURL, err)
			continue
		}

		// 读取响应内容
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			fmt.Printf("读取IPv4 API %s响应失败: %v\n", apiURL, err)
			continue
		}

		responseStr := string(body)
		fmt.Printf("IPv4 API %s返回状态码: %d, 返回值: %s\n", apiURL, resp.StatusCode, responseStr)

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("IPv4 API %s返回错误状态码: %d\n", apiURL, resp.StatusCode)
			continue
		}

		var ip string
		if strings.Contains(apiURL, "format=json") {
			var ipResp IPResponse
			if err := json.Unmarshal(body, &ipResp); err != nil {
				fmt.Printf("解析IPv4 API %s响应失败: %v\n", apiURL, err)
				continue
			}
			ip = ipResp.IP
		} else {
			// 处理纯文本响应
			ip = strings.TrimSpace(responseStr)
		}

		if ip != "" {
			// 检查IPv4地址的有效性
			if isValidIPv4(ip) {
				fmt.Printf("成功从IPv4 API %s获取到有效的IP地址: %s\n", apiURL, ip)
				return ip, nil
			} else {
				fmt.Printf("IPv4 API %s返回的IP地址无效: %s\n", apiURL, ip)
				continue
			}
		} else {
			fmt.Printf("IPv4 API %s返回空IP地址\n", apiURL)
			continue
		}
	}

	fmt.Println("遍历完所有IPv4 API，均未能成功获取IP地址")
	return "", fmt.Errorf("所有IPv4 API都失败了")
}

// getIPv6Address 获取公网IPv6地址
func getIPv6Address() (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 从配置文件中获取IPv6地址API列表，如果为空则使用默认列表
	apis := appConfig.IPv6List
	if len(apis) == 0 {
		// 使用默认的IPv6地址API列表
		apis = []string{
			"https://api6.ipify.org?format=json",
			"https://ifconfig.me/ip",
			"https://ipecho.net/plain",
			"https://api.ip.sb/ip",
			"https://ipinfo.io/ip",
			"https://checkip.amazonaws.com",
			"https://ident.me",
			"https://bot.whatismyipaddress.com",
			"https://myexternalip.com/raw",
			"https://ipaddr.site",
		}
		fmt.Println("使用默认IPv6地址API列表")
	}

	fmt.Println("开始遍历IPv6地址API清单...")

	for i, apiURL := range apis {
		fmt.Printf("正在尝试第%d个IPv6 API: %s\n", i+1, apiURL)

		resp, err := client.Get(apiURL)
		if err != nil {
			fmt.Printf("尝试IPv6 API %s失败: %v\n", apiURL, err)
			continue
		}

		// 读取响应内容
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			fmt.Printf("读取IPv6 API %s响应失败: %v\n", apiURL, err)
			continue
		}

		responseStr := string(body)
		fmt.Printf("IPv6 API %s返回状态码: %d, 返回值: %s\n", apiURL, resp.StatusCode, responseStr)

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("IPv6 API %s返回错误状态码: %d\n", apiURL, resp.StatusCode)
			continue
		}

		var ip string
		if strings.Contains(apiURL, "format=json") {
			var ipResp IPResponse
			if err := json.Unmarshal(body, &ipResp); err != nil {
				fmt.Printf("解析IPv6 API %s响应失败: %v\n", apiURL, err)
				continue
			}
			ip = ipResp.IP
		} else {
			// 处理纯文本响应
			ip = strings.TrimSpace(responseStr)
		}

		if ip != "" {
			fmt.Printf("成功从IPv6 API %s获取到IP地址: %s\n", apiURL, ip)
			return ip, nil
		} else {
			fmt.Printf("IPv6 API %s返回空IP地址\n", apiURL)
			continue
		}
	}

	fmt.Println("遍历完所有IPv6 API，均未能成功获取IP地址")
	return "", fmt.Errorf("所有IPv6 API都失败了")
}

// sendEmail 发送邮件
func sendEmail(smtpServer, smtpPort, username, password, from, to, subject, body, senderName string, sendMode int) error {
	fmt.Printf("准备发送邮件...\n")
	fmt.Printf("SMTP服务器: %s:%s\n", smtpServer, smtpPort)
	fmt.Printf("发件人: %s\n", from)
	fmt.Printf("发件人名称: %s\n", senderName)
	fmt.Printf("收件人: %s\n", to)
	fmt.Printf("邮件主题: %s\n", subject)
	fmt.Printf("邮件内容: %s\n", body)

	sendMsg = true

	// 验证配置
	if smtpServer == "" || smtpPort == "" || username == "" || password == "" || from == "" || to == "" {
		return fmt.Errorf("邮件配置不完整")
	}

	// 如果senderName为空，使用username作为默认值
	if senderName == "" {
		senderName = username
	}

	fmt.Println("正在连接SMTP服务器: " + smtpServer + "; username: " + username)
	auth := smtp.PlainAuth("", username, password, smtpServer)

	// 构建邮件头，支持HTML格式
	headers := make(map[string]string)
	headers["From"] = fmt.Sprintf("%s <%s>", senderName, from)
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "text/html; charset=\"utf-8\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	aaddr := fmt.Sprintf("%s:%s", smtpServer, smtpPort)

	var toList []string
	if sendMode == 1 {
		toList = strings.Split(to, ",")
		for i := range toList {
			toList[i] = strings.TrimSpace(toList[i])
		}
	} else {
		toList = []string{to}
	}

	fmt.Println("正在发送邮件...")
	err := smtp.SendMail(aaddr, auth, from, toList, []byte(message))
	if err != nil {
		fmt.Printf("邮件发送失败: %v\n", err)
		return err
	}

	fmt.Println("邮件发送成功！")
	return nil
}

// checkIPChanges 检查IP地址变化并发送通知
func checkIPChanges() {
	fmt.Println("\n开始检查IP地址变化...")

	runCount++

	// 获取可执行文件所在目录
	var err error
	execPath, err = getExecutablePath()
	if err != nil {
		fmt.Printf("获取程序路径失败: %v\n", err)
		return
	}
	execDir = filepath.Dir(execPath)

	// 获取当前IPv4地址
	currentIPv4, err := getIPv4Address()
	if err != nil {
		fmt.Printf("获取IPv4地址失败: %v\n", err)
	} else {
		fmt.Printf("当前IPv4地址: %s\n", currentIPv4)
	}

	// 获取当前IPv6地址
	currentIPv6, err := getIPv6Address()
	if err != nil {
		fmt.Printf("获取IPv6地址失败: %v\n", err)
	} else {
		fmt.Printf("当前IPv6地址: %s\n", currentIPv6)
	}

	// 检查是否有变化
	hasChanged := false
	var changeDetails string

	if currentIPv4 != "" && currentIPv4 != cachedIPv4 {
		hasChanged = true
		if cachedIPv4 == "" {
			changeDetails += fmt.Sprintf("IPv4地址: 首次检测到 %s\n", currentIPv4)
		} else {
			changeDetails += fmt.Sprintf("IPv4地址: %s → %s\n", cachedIPv4, currentIPv4)
		}
		cachedIPv4 = currentIPv4
	}

	if currentIPv6 != "" && currentIPv6 != cachedIPv6 {
		hasChanged = true
		if cachedIPv6 == "" {
			changeDetails += fmt.Sprintf("IPv6地址: 首次检测到 %s\n", currentIPv6)
		} else {
			changeDetails += fmt.Sprintf("IPv6地址: %s → %s\n", cachedIPv6, currentIPv6)
		}
		cachedIPv6 = currentIPv6
	}

	// 检查是否两个地址都为空
	bothEmpty := currentIPv4 == "" && currentIPv6 == ""

	// 创建运行记录
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	record := &RunRecord{
		LastRunTime: currentTime,
		IPv4:        currentIPv4,
		IPv6:        currentIPv6,
		EmailSent:   false,
		RunCount:    runCount,
	}

	// 如果有变化或两个地址都为空，发送邮件通知
	if hasChanged || bothEmpty {
		fmt.Println("准备发送邮件通知...")

		var subject string
		var body string

		title := appConfig.MailConfig.Title
		if title == "" {
			title = "公网IP地址监测"
		}

		if bothEmpty {
			subject = fmt.Sprintf("%s - 异常通知 - %s", title, currentTime)
			body = fmt.Sprintf("检测到公网IP地址异常：无法获取到任何IP地址\n\n检测时间: %s", currentTime)
			fmt.Println("检测到IP地址异常：无法获取到任何IP地址")
		} else {
			subject = fmt.Sprintf("%s - 变更通知 - %s", title, currentTime)

			templateData := &TemplateData{
				ExecPath:     execPath,
				ProgramPID:   programPID,
				FirstRunTime: firstRunTime,
				LastRunTime:  lastRunTime,
				RunCount:     runCount,
				SendMsg:      sendMsg,
				LastRecord:   runRecord, // 已废弃，保留用于兼容性
				CurrentTime:  currentTime,
				CurrentIPv4:  currentIPv4,
				CurrentIPv6:  currentIPv6,
			}

			var err error
			templatePath := filepath.Join(execDir, "mail_template.html")
			body, err = renderMailTemplate(templatePath, templateData)
			if err != nil {
				fmt.Printf("渲染邮件模板失败: %v\n", err)
				body = fmt.Sprintf("检测到公网IP地址发生变更:\n\n%s\n\n检测时间: %s", changeDetails, currentTime)
			}
			fmt.Println("检测到IP地址变化，准备发送邮件通知...")
		}

		// 使用goroutine异步发送邮件，避免阻塞定时任务
		go func() {
			sendErr := sendEmail(appConfig.MailConfig.SmtpServer, appConfig.MailConfig.SmtpPort, appConfig.MailConfig.Username, appConfig.MailConfig.Password, appConfig.MailConfig.From, appConfig.MailConfig.To, subject, body, appConfig.MailConfig.SenderName, appConfig.MailConfig.SendMode)
			if sendErr != nil {
				fmt.Printf("邮件发送失败: %v\n", sendErr)
			} else {
				fmt.Println("邮件发送成功！")
			}
		}()

		// 异步发送邮件，直接标记为已发送（实际发送结果会在goroutine中输出）
		record.EmailSent = true
		record.EmailResult = "异步发送中"
	} else {
		fmt.Println("未检测到IP地址变化")
		record.EmailSent = false
		record.EmailResult = "未发送（IP地址未变化）"
	}

	// 更新上一次运行时间
	lastRunTime = currentTime
}

// getExecutablePath 获取当前可执行文件的完整路径
func getExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(execPath)
}

// isWindows 检测是否为Windows系统
func isWindows() bool {
	return runtime.GOOS == "windows"
}

// isLinux 检测是否为Linux系统
func isLinux() bool {
	return runtime.GOOS == "linux"
}

// isDarwin 检测是否为macOS系统
func isDarwin() bool {
	return runtime.GOOS == "darwin"
}

// installWindowsService 安装Windows服务
func installWindowsService() error {
	execPath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %v", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	serviceName := "public_ip_monitor"
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("服务 %s 已存在", serviceName)
	}

	s, err = m.CreateService(serviceName, execPath, mgr.Config{
		DisplayName: "Public IP Monitor",
		Description: "监控公网IP地址变化并发送邮件通知",
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("创建服务失败: %v", err)
	}
	defer s.Close()

	fmt.Printf("已安装Windows服务: %s\n", serviceName)
	fmt.Printf("服务路径: %s\n", execPath)
	fmt.Printf("启动类型: 自动\n")
	return nil
}

// startWindowsService 启动Windows服务
func startWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	serviceName := "public_ip_monitor"
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("打开服务失败: %v", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("查询服务状态失败: %v", err)
	}

	if status.State == svc.Running {
		fmt.Println("服务已在运行中")
		return nil
	}

	err = s.Start()
	if err != nil {
		return fmt.Errorf("启动服务失败: %v", err)
	}

	fmt.Printf("已启动服务: %s\n", serviceName)
	return nil
}

// uninstallWindowsService 卸载Windows服务
func uninstallWindowsService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	serviceName := "public_ip_monitor"
	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("打开服务失败: %v", err)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("查询服务状态失败: %v", err)
	}

	if status.State != svc.Stopped {
		_, err = s.Control(svc.Stop)
		if err != nil {
			return fmt.Errorf("停止服务失败: %v", err)
		}

		for i := 0; i < 10; i++ {
			time.Sleep(1 * time.Second)
			status, err = s.Query()
			if err != nil {
				return fmt.Errorf("查询服务状态失败: %v", err)
			}
			if status.State == svc.Stopped {
				break
			}
		}
	}

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("删除服务失败: %v", err)
	}

	fmt.Printf("已卸载Windows服务: %s\n", serviceName)
	return nil
}

// isWindowsServiceInstalled 检查Windows服务是否已安装
func isWindowsServiceInstalled() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("连接服务管理器失败: %v", err)
	}
	defer m.Disconnect()

	serviceName := "public_ip_monitor"
	s, err := m.OpenService(serviceName)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "指定的服务未安装") {
			return false, nil
		}
		return false, err
	}
	defer s.Close()

	return true, nil
}

// ipMonitorService Windows服务实现
type ipMonitorService struct{}

// Execute 实现svc.Handler接口
func (s *ipMonitorService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// 初始化监控逻辑
	initializeMonitoring()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				break loop
			case svc.Pause:
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				fmt.Printf("意外的服务请求: %v\n", c)
			}
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	return false, 0
}

// initializeMonitoring 初始化监控逻辑
func initializeMonitoring() {
	// 不再读取run_record.json文件，每次启动都是全新运行
	runRecord = nil

	// 初始化运行次数和运行时间
	runCount = 0
	firstRunTime = ""
	lastRunTime = ""

	sendMsg = false

	// 获取程序路径和进程ID
	var err error
	execPath, err = getExecutablePath()
	if err != nil {
		execPath = "未知路径"
	}
	programPID = os.Getpid()

	// 获取可执行文件所在目录
	execDir := filepath.Dir(execPath)

	// 加载配置文件
	configPath := filepath.Join(execDir, "config.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		return
	}
	appConfig = config

	// 初始化缓存的IP地址
	cachedIPv4, _ = getIPv4Address()
	cachedIPv6, _ = getIPv6Address()

	// 首次运行，发送初始IP地址通知邮件
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	title := appConfig.MailConfig.Title
	if title == "" {
		title = "公网IP地址监测"
	}
	subject := fmt.Sprintf("%s - 初始通知 - %s", title, currentTime)

	if runRecord == nil {
		firstRunTime = currentTime
	}

	var body string

	templateData := &TemplateData{
		ExecPath:     execPath,
		ProgramPID:   programPID,
		FirstRunTime: firstRunTime,
		LastRunTime:  lastRunTime,
		RunCount:     runCount,
		SendMsg:      sendMsg,
		LastRecord:   runRecord,
		CurrentTime:  currentTime,
		CurrentIPv4:  cachedIPv4,
		CurrentIPv6:  cachedIPv6,
	}

	templatePath := filepath.Join(execDir, "mail_template.html")
	body, err = renderMailTemplate(templatePath, templateData)
	if err != nil {
		body = fmt.Sprintf("首次运行，发送初始IP地址通知\n\nIPv4: %s\nIPv6: %s", cachedIPv4, cachedIPv6)
	}

	// 使用goroutine异步发送初始邮件，避免阻塞程序启动
	go func() {
		sendEmail(appConfig.MailConfig.SmtpServer, appConfig.MailConfig.SmtpPort, appConfig.MailConfig.Username, appConfig.MailConfig.Password, appConfig.MailConfig.From, appConfig.MailConfig.To, subject, body, appConfig.MailConfig.SenderName, appConfig.MailConfig.SendMode)
	}()

	c := cron.New()
	c.AddFunc(appConfig.TaskPara.CronExpression, checkIPChanges)
	c.Start()
}

// addToWindowsStartup 添加到Windows开机启动（已废弃，保留用于兼容性）
func addToWindowsStartup() error {
	return installWindowsService()
}

// removeFromWindowsStartup 从Windows开机启动中移除（已废弃，保留用于兼容性）
func removeFromWindowsStartup() error {
	return uninstallWindowsService()
}

// addToLinuxStartup 添加到Linux开机启动（systemd）
func addToLinuxStartup() error {
	execPath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %v", err)
	}

	serviceContent := fmt.Sprintf(`[Unit]
Description=IP Monitor Service
After=network.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`, execPath)

	servicePath := "/etc/systemd/system/ip-monitor.service"
	err = ioutil.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("写入systemd服务文件失败: %v", err)
	}

	cmd := exec.Command("systemctl", "daemon-reload")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("重载systemd失败: %v", err)
	}

	cmd = exec.Command("systemctl", "enable", "ip-monitor.service")
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("启用服务失败: %v", err)
	}

	fmt.Printf("已添加到Linux开机启动: %s\n", execPath)
	return nil
}

// removeFromLinuxStartup 从Linux开机启动中移除
func removeFromLinuxStartup() error {
	servicePath := "/etc/systemd/system/ip-monitor.service"

	cmd := exec.Command("systemctl", "stop", "ip-monitor.service")
	_ = cmd.Run()

	cmd = exec.Command("systemctl", "disable", "ip-monitor.service")
	_ = cmd.Run()

	err := os.Remove(servicePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除服务文件失败: %v", err)
	}

	cmd = exec.Command("systemctl", "daemon-reload")
	_ = cmd.Run()

	fmt.Println("已从Linux开机启动中移除")
	return nil
}

// addToDarwinStartup 添加到macOS开机启动（LaunchAgent）
func addToDarwinStartup() error {
	execPath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %v", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	err = os.MkdirAll(launchAgentsDir, 0755)
	if err != nil {
		return fmt.Errorf("创建LaunchAgents目录失败: %v", err)
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.ip-monitor</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/ip-monitor.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/ip-monitor.err</string>
</dict>
</plist>
`, execPath)

	plistPath := filepath.Join(launchAgentsDir, "com.user.ip-monitor.plist")
	err = ioutil.WriteFile(plistPath, []byte(plistContent), 0644)
	if err != nil {
		return fmt.Errorf("写入LaunchAgent plist文件失败: %v", err)
	}

	cmd := exec.Command("launchctl", "load", plistPath)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("加载LaunchAgent失败: %v", err)
	}

	fmt.Printf("已添加到macOS开机启动: %s\n", execPath)
	return nil
}

// removeFromDarwinStartup 从macOS开机启动中移除
func removeFromDarwinStartup() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录失败: %v", err)
	}

	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.user.ip-monitor.plist")

	cmd := exec.Command("launchctl", "unload", plistPath)
	_ = cmd.Run()

	err = os.Remove(plistPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除LaunchAgent plist文件失败: %v", err)
	}

	fmt.Println("已从macOS开机启动中移除")
	return nil
}

// addToStartup 添加到开机启动
func addToStartup() error {
	if isWindows() {
		return addToWindowsStartup()
	} else if isLinux() {
		return addToLinuxStartup()
	} else if isDarwin() {
		return addToDarwinStartup()
	}
	return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
}

// removeFromStartup 从开机启动中移除
func removeFromStartup() error {
	if isWindows() {
		return removeFromWindowsStartup()
	} else if isLinux() {
		return removeFromLinuxStartup()
	} else if isDarwin() {
		return removeFromDarwinStartup()
	}
	return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
}

// checkAndAddToStartup 检查并添加到开机启动
func checkAndAddToStartup() error {
	if isWindows() {
		installed, err := isWindowsServiceInstalled()
		if err != nil {
			return fmt.Errorf("检查服务状态失败: %v", err)
		}

		if installed {
			fmt.Println("Windows服务已安装")
			err = startWindowsService()
			if err != nil {
				return fmt.Errorf("启动服务失败: %v", err)
			}
			return nil
		}

		err = installWindowsService()
		if err != nil {
			return err
		}

		return startWindowsService()
	} else if isLinux() {
		cmd := exec.Command("systemctl", "is-enabled", "ip-monitor.service")
		err := cmd.Run()
		if err == nil {
			fmt.Println("程序已在开机启动中")
			return nil
		}

		return addToLinuxStartup()
	} else if isDarwin() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户主目录失败: %v", err)
		}

		plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", "com.user.ip-monitor.plist")
		if _, err := os.Stat(plistPath); err == nil {
			cmd := exec.Command("launchctl", "list", "com.user.ip-monitor")
			if cmd.Run() == nil {
				fmt.Println("程序已在开机启动中")
				return nil
			}
		}

		return addToDarwinStartup()
	}

	return fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
}

func main() {
	if isWindows() {
		isIntSess, err := svc.IsWindowsService()
		if err == nil && isIntSess {
			runService()
			return
		}
	}

	runApplication()
}

// runService 以服务模式运行
func runService() {
	runsvc := &ipMonitorService{}
	err := svc.Run("public_ip_monitor", runsvc)
	if err != nil {
		fmt.Printf("服务运行失败: %v\n", err)
		return
	}
}

// runApplication 以应用程序模式运行
func runApplication() {
	// 检查命令行参数
	if len(os.Args) > 1 {
		if os.Args[1] == "--uninstall" {
			fmt.Println("正在卸载Windows服务...")
			err := uninstallWindowsService()
			if err != nil {
				fmt.Printf("卸载失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("卸载成功！")
			os.Exit(0)
		}
	}

	// 检查并添加到开机启动
	fmt.Println("检查Windows服务状态...")
	err := checkAndAddToStartup()
	if err != nil {
		fmt.Printf("安装Windows服务失败: %v\n", err)
		fmt.Println("程序将继续运行，但不会自动开机启动")
	}

	// 获取可执行文件所在目录
	execDir = filepath.Dir(execPath)

	// 不再读取run_record.json文件，每次启动都是全新运行
	runRecord = nil

	// 初始化运行次数和运行时间
	runCount = 0
	firstRunTime = ""
	lastRunTime = ""

	sendMsg = false

	// 输出上一次运行日志（首次运行时显示提示信息）
	printLastRunLog(runRecord)

	// 获取程序路径和进程ID
	execPath, err = getExecutablePath()
	if err != nil {
		fmt.Printf("获取程序路径失败: %v\n", err)
		execPath = "未知路径"
	}
	programPID = os.Getpid()
	fmt.Printf("程序路径: %s\n", execPath)
	fmt.Printf("进程ID: %d\n", programPID)

	// 获取可执行文件所在目录
	execDir := filepath.Dir(execPath)

	// 加载配置文件
	configPath := filepath.Join(execDir, "config.yaml")
	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Printf("警告: %v，请检查config.yaml配置文件\n", err)
		os.Exit(1)
	}
	appConfig = config

	// 初始化缓存的IP地址
	fmt.Println("初始化IP地址缓存...")
	cachedIPv4, _ = getIPv4Address()
	cachedIPv6, _ = getIPv6Address()

	fmt.Printf("初始化IPv4地址: %s\n", cachedIPv4)
	fmt.Printf("初始化IPv6地址: %s\n", cachedIPv6)

	// 首次运行，发送初始IP地址通知邮件
	fmt.Println("首次运行，发送初始IP地址通知邮件...")
	currentTime := time.Now().Format("2006-01-02 15:04:05")
	title := appConfig.MailConfig.Title
	if title == "" {
		title = "公网IP地址监测"
	}
	subject := fmt.Sprintf("%s - 初始通知 - %s", title, currentTime)

	if runRecord == nil {
		firstRunTime = currentTime
	}

	var body string

	templateData := &TemplateData{
		ExecPath:     execPath,
		ProgramPID:   programPID,
		FirstRunTime: firstRunTime,
		LastRunTime:  lastRunTime,
		RunCount:     runCount,
		SendMsg:      sendMsg,
		LastRecord:   runRecord, // 已废弃，保留用于兼容性
		CurrentTime:  currentTime,
		CurrentIPv4:  cachedIPv4,
		CurrentIPv6:  cachedIPv6,
	}

	templatePath := filepath.Join(execDir, "mail_template.html")
	body, err = renderMailTemplate(templatePath, templateData)
	if err != nil {
		fmt.Printf("渲染邮件模板失败: %v\n", err)
		body = fmt.Sprintf("首次运行，发送初始IP地址通知\n\nIPv4: %s\nIPv6: %s", cachedIPv4, cachedIPv6)
	}

	// 使用goroutine异步发送初始邮件，避免阻塞程序启动
	go func() {
		sendErr := sendEmail(appConfig.MailConfig.SmtpServer, appConfig.MailConfig.SmtpPort, appConfig.MailConfig.Username, appConfig.MailConfig.Password, appConfig.MailConfig.From, appConfig.MailConfig.To, subject, body, appConfig.MailConfig.SenderName, appConfig.MailConfig.SendMode)
		if sendErr != nil {
			fmt.Printf("邮件发送失败: %v\n", sendErr)
		} else {
			fmt.Println("邮件发送成功！")
		}
	}()

	c := cron.New()
	c.AddFunc(appConfig.TaskPara.CronExpression, checkIPChanges)
	fmt.Printf("定时检查已启动，Cron表达式: %s\n", appConfig.TaskPara.CronExpression)

	c.Start()

	select {}
}
