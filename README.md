# IP监控程序

## 使用基于VSCode的Trae-CN开发工具，统计代码构成

本项目代码构成如下：

| 文件类型 | 文件名 | 行数 | 说明 |
|---------|--------|------|------|
| 个人编写(实为自动) | main.go | 975 | 主程序逻辑 |
| 个人编写 (实为自动) | template.go | 122 | 邮件模板处理 |
| 个人编写 (部分自动) | mail_template.html | 65 | 邮件HTML模板 |
| 自动生成 | go.mod | 12 | Go模块依赖管理 |
| 自动生成 | go.sum | 7 | Go依赖校验和 |
| **总计** | **5个文件** | **1181** | **100%** |

**个人调整统计代码占比（符合真实情况，前期没有使用git，不会超出5%）：**
- 自动编写代码：1162行（98.4%）
- 个人生成代码：19行（1.6%）

## 功能介绍

这是一个公网IP地址监控程序，用于监控IPv4和IPv6地址的变化，并通过邮件发送通知。

## 主要功能

### 1. IP地址监控
- 支持IPv4和IPv6地址监控
- 使用多个API接口获取公网IP地址，提高可靠性
- 检测到IP地址变化时自动发送邮件通知

### 2. 定时检查
- 使用cron表达式配置检查频率
- 支持灵活的时间配置（如每分钟、每小时、每天等）

### 3. 开机启动
- **Windows平台**：通过注册表 `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run` 添加开机启动项
- **Linux平台**：通过systemd服务实现开机启动
- **macOS平台**：通过LaunchAgent实现开机启动
- 程序启动时自动检测并添加到开机启动（如果未添加）
- **配置文件路径**：程序使用可执行文件所在目录作为基准路径读取配置文件，确保开机启动时能正确读取配置文件

### 4. 卸载功能
- 支持 `--uninstall` 参数从开机启动中移除程序
- Windows：从注册表中删除启动项
- Linux：停止并禁用systemd服务，删除服务文件
- macOS：使用launchctl卸载LaunchAgent，删除plist文件

### 5. 累计运行次数
- 记录程序累计运行次数
- 每次检查IP地址时自动累加
- 在邮件中显示累计运行次数
- 程序启动时累计运行次数从0开始

### 6. 运行时间记录
- **第一次运行时间**：程序启动的时间
- **上一次运行时间**：每次检查IP地址时更新
- 如果是第1次运行，则上一次运行时间为空

### 7. HTML格式邮件
- 邮件支持HTML格式
- 采用了类似MyBatis中类似if-test标签的风格,**套用了ONGL表达式来扩展HTML输出的定义**
- 程序名称、路径、进程ID使用红色加粗字体显示
- 邮件包含程序信息、运行时间信息和当前IP地址信息

## 配置文件

### config.yaml
程序配置文件，包含以下4个节点：

```yaml
# 邮件参数配置
mail-config:
  smtp_server: smtp.126.com
  smtp_port: "25"
  username: your@email.com
  password: your_password_or_auth_code
  from: your@email.com
  to: recipient1@email.com,recipient2@email.com
  send_mode: 1  # 1-单个发送，3-群发

# IPv4地址API列表
ip-v4-list:
  - https://ipinfo.io/ip
  - https://api.ipify.org?format=json&ipv4=true
  - https://ifconfig.me/ip
  - https://ipecho.net/plain
  - https://api.ip.sb/ip
  - https://checkip.amazonaws.com
  - https://ident.me
  - https://bot.whatismyipaddress.com
  - https://myexternalip.com/raw
  - https://ipaddr.site

# IPv6地址API列表
ip-v6-list:
  - https://api6.ipify.org?format=json
  - https://ifconfig.me/ip
  - https://ipecho.net/plain
  - https://api.ip.sb/ip
  - https://ipinfo.io/ip
  - https://checkip.amazonaws.com
  - https://ident.me
  - https://bot.whatismyipaddress.com
  - https://myexternalip.com/raw
  - https://ipaddr.site

# 任务参数配置
task-para:
  cron_expression: "*/1 * * * *"  # 每分钟检查一次
```

**配置说明：**

#### mail-config（原json邮件参数配置，已废弃，由yaml文件替换）
- `smtp_server`: SMTP服务器地址
- `smtp_port`: SMTP服务器端口
- `username`: SMTP用户名（邮箱地址）
- `password`: SMTP密码（邮箱授权码）
- `from`: 发件人邮箱地址
- `to`: 收件人邮箱地址（多个地址用英文逗号分隔）
- `send_mode`: 邮件发送模式
  - `1` - 单个发送模式（多个收件人用逗号分隔，逐个发送）
  - `3` - 群发模式（所有收件人一起发送）

#### ip-v4-list（IPv4地址API列表）
- IPv4地址查询API的URL列表
- 如果配置文件中为空，则使用程序中的默认列表
- 程序会按顺序尝试这些API，直到成功获取到IP地址

#### ip-v6-list（IPv6地址API列表）
- IPv6地址查询API的URL列表
- 如果配置文件中为空，则使用程序中的默认列表
- 程序会按顺序尝试这些API，直到成功获取到IP地址

#### task-para（任务参数配置）
- `cron_expression`: cron表达式，定义IP检查频率
  - `* * * * *` - 每分钟检查一次
  - `*/2 * * * *` - 每2分钟检查一次
  - `0 * * * *` - 每小时检查一次
  - `0 9 * * *` - 每天上午9点检查一次

## 使用方法

### 正常运行
```bash
# Windows
ip-monitor.exe

# Linux
./ip-monitor
```

### 卸载开机启动
```bash
# Windows
ip-monitor.exe --uninstall

# Linux
./ip-monitor --uninstall

# macOS
./ip-monitor --uninstall
```

## 邮件内容说明

### 邮件发送模式
程序支持两种邮件发送模式：
- **单个发送模式（send_mode = 1）**：多个收件人用英文逗号分隔，逐个发送邮件
- **群发模式（send_mode = 3）**：所有收件人一起发送邮件

### 首次运行邮件
- **程序名称**：红色加粗显示
- **程序路径**：红色加粗显示
- **进程ID**：红色加粗显示
- **第一次运行时间**：红色加粗显示当前时间（程序启动时间）
- **上一次运行时间**：红色加粗显示"无"
- **累计运行次数**：红色加粗显示0
- 显示"第一次运行该程序，首次发送邮件"
- 邮件模板包含2个部分：运行程序的消息、当前IP地址信息

### 第二次运行邮件
- **程序名称**：红色加粗显示
- **程序路径**：红色加粗显示
- **进程ID**：红色加粗显示
- **第一次运行时间**：红色加粗显示第一次运行时间（程序启动时间）
- **上一次运行时间**：红色加粗显示"无"
- **累计运行次数**：红色加粗显示1
- 显示"程序重新启动"
- 邮件模板包含2个部分：运行程序的消息、当前IP地址信息

### 第三次及以后运行邮件
- **程序名称**：红色加粗显示
- **程序路径**：红色加粗显示
- **进程ID**：红色加粗显示
- **第一次运行时间**：红色加粗显示第一次运行时间（程序启动时间）
- **上一次运行时间**：红色加粗显示上一次运行时间（上一次检查IP地址的时间）
- **累计运行次数**：红色加粗显示累计次数
- 显示"程序重新启动"
- 邮件模板包含3个部分：运行程序的消息、上一次运行日志、最后1次运行信息

## 支持的平台

- Windows (推荐)
- Linux (推荐)
- macOS (推荐)

## 依赖项

- Go 1.24.13 或更高版本
- 网络连接（用于访问IP查询API）
- SMTP服务器（用于发送邮件通知）

## 编译

```bash
go build -o ip-monitor.exe
```

## 注意事项

1. 首次运行时，程序会自动添加到开机启动
2. 使用 `--uninstall` 参数可以从开机启动中移除程序
3. 邮件配置需要使用正确的SMTP服务器和授权码
4. 累计运行次数会在每次检查IP地址时自动累加
5. 邮件内容使用HTML格式，建议使用支持HTML的邮件客户端查看
6. **第一次运行时间**：程序启动的时间
7. **上一次运行时间**：每次检查IP地址时更新
8. **邮件发送模式**：
   - 单个发送模式（send_mode = 1）：多个收件人用英文逗号分隔，逐个发送邮件
   - 群发模式（send_mode = 3）：所有收件人一起发送邮件
10. **邮件模板**：
    - 首次运行：包含2个部分（运行程序的消息、当前IP地址信息）
    - 第二次运行：包含2个部分（运行程序的消息、当前IP地址信息）
    - 第三次及以后：包含3个部分（运行程序的消息、上一次运行日志、最后1次运行信息）
11. **配置文件格式**：
    - 使用YAML格式的配置文件 `config.yaml`
    - 配置文件分为4个节点：mail-config、ip-v4-list、ip-v6-list、task-para
    - IPv4和IPv6的API列表可以在配置文件中自定义，如果为空则使用程序中的默认列表
    - 程序会按顺序尝试API列表，直到成功获取到IP地址

## 版本历史

### v1.4.0
- 修复开机启动时无法读取配置文件的问题
- 使用可执行文件所在目录作为基准路径读取配置文件
- 确保配置文件、运行记录、邮件模板等文件路径正确
- 支持Windows、Linux、macOS三个平台的开机启动配置文件读取
- ** 增加发送邮件html模板配置文件mail_template.html**，支持mybatis 中 if-test判断技术特性，将业务规则从程序中的流程判断，前置html配置文件中。

### v1.3.0
- 添加macOS平台开机启动支持
- 使用LaunchAgent实现macOS开机启动
- 支持macOS平台的卸载功能
- 完善跨平台支持（Windows、Linux、macOS）

### v1.2.0
- 配置文件格式从JSON改为YAML
- 配置文件分为4个节点：mail-config、ip-v4-list、ip-v6-list、task-para
- IPv4和IPv6的API列表可以在配置文件中自定义
- 如果配置文件中API列表为空，则使用程序中的默认列表
- 添加gopkg.in/yaml.v3依赖支持YAML解析
- 更新.gitignore文件，忽略config.yaml配置文件
- 创建config.example.yaml示例配置文件

### v1.1.0
- 添加邮件发送模式支持（单个发送和群发）
- 优化运行时间记录逻辑（第一次运行时间为程序启动时间，上一次运行时间为每次检查IP地址时更新）
- 完善代码注释和文档说明

### v1.0.0
- 初始版本
- 支持IPv4和IPv6地址监控
- 支持开机启动和卸载功能
- 支持运行记录和累计运行次数
- 支持HTML格式邮件


## V2.0.0 增加功能(计划中)
### 为了保障开源软件的可持续能力建设，License修订，从Mit协议修改改为按年付费的服务模式
### 短信模板配置、发送功能
### 增加端侧的ip监控，逐渐覆盖包括手机端、嵌入式设备、物联网终端、视频监控、车载芯片等端侧设备
### 日志优化和监控能力优化
### AI新技术探索


## V3.0.0 服务与框架解藕(计划中)
### 架构上解构
核心运行程序服务化，客户端提供框架，可以动态加载服务器端下发的服务。
思路：客户端即端侧()，需要加载框架；服务器端提供各类服务，由客户端来进行订阅。
### AI+能力正式引入
### 持续完善V2.0.0的优化，主要包括性能、分布式架构优化
