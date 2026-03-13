package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
)

// TemplateData 存储邮件模板渲染所需的数据结构
type TemplateData struct {
	ExecPath     string     // 可执行文件路径
	ProgramPID   int        // 程序进程ID
	FirstRunTime string     // 首次运行时间
	LastRunTime  string     // 上一次运行时间
	RunCount     int        // 累计运行次数
	SendMsg      bool       // 是否发送了邮件
	LastRecord   *RunRecord // 上一次运行记录
	CurrentTime  string     // 当前时间
	CurrentIPv4  string     // 当前IPv4地址
	CurrentIPv6  string     // 当前IPv6地址
}

// evaluateCondition 评估条件表达式
// 参数:
//
//	condition: 条件表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	bool: 条件是否为真
func evaluateCondition(condition string, data *TemplateData) bool {
	condition = strings.TrimSpace(condition)
	return evaluateExpression(condition, data)
}

// evaluateExpression 评估复杂表达式，支持and、or操作符和括号组合
// 参数:
//
//	expression: 表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	bool: 表达式是否为真
func evaluateExpression(expression string, data *TemplateData) bool {
	// 移除所有空格以简化解析
	expression = strings.ReplaceAll(expression, " ", "")

	// 处理括号
	for strings.Contains(expression, "(") {
		// 找到最内层的括号
		start := strings.LastIndex(expression, "(")
		end := strings.Index(expression[start:], ")") + start
		if start == -1 || end == -1 {
			return false
		}

		// 计算括号内的表达式
		innerExpr := expression[start+1 : end]
		result := evaluateExpression(innerExpr, data)

		// 替换括号为结果
		expression = expression[:start] + strconv.FormatBool(result) + expression[end+1:]
	}

	// 处理OR操作符
	if strings.Contains(expression, "or") {
		parts := strings.Split(expression, "or")
		for _, part := range parts {
			if evaluateExpression(part, data) {
				return true
			}
		}
		return false
	}

	// 处理AND操作符
	if strings.Contains(expression, "and") {
		parts := strings.Split(expression, "and")
		for _, part := range parts {
			if !evaluateExpression(part, data) {
				return false
			}
		}
		return true
	}

	// 处理比较操作
	return evaluateComparison(expression, data)
}

// evaluateComparison 评估比较操作表达式
// 参数:
//
//	comparison: 比较表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	bool: 比较结果是否为真
func evaluateComparison(comparison string, data *TemplateData) bool {
	// 支持的比较操作符
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if strings.Contains(comparison, op) {
			parts := strings.Split(comparison, op)
			if len(parts) != 2 {
				return false
			}

			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])

			return evaluateCompare(left, right, op, data)
		}
	}

	// 处理布尔值
	if comparison == "true" {
		return true
	}
	if comparison == "false" {
		return false
	}

	// 处理变量
	if comparison == "lastRecord==nil" {
		return data.LastRecord == nil
	}
	if comparison == "lastRecord" {
		return data.LastRecord != nil
	}

	return false
}

// evaluateCompare 执行具体的比较操作
// 参数:
//
//	left: 左侧表达式
//	right: 右侧表达式
//	op: 比较操作符
//	data: 模板数据
//
// 返回值:
//
//	bool: 比较结果是否为真
func evaluateCompare(left, right, op string, data *TemplateData) bool {
	// 对于RunCount的比较，需要特殊处理
	if left == "RunCount" {
		leftInt, err := strconv.Atoi(right)
		if err == nil {
			switch op {
			case "==":
				return data.RunCount == leftInt
			case "!=":
				return data.RunCount != leftInt
			case ">":
				return data.RunCount > leftInt
			case "<":
				return data.RunCount < leftInt
			case ">=":
				return data.RunCount >= leftInt
			case "<=":
				return data.RunCount <= leftInt
			}
		}
	}

	// 对于其他比较
	leftVal := left
	rightVal := right

	switch op {
	case "==":
		return leftVal == rightVal
	case "!=":
		return leftVal != rightVal
	default:
		return false
	}
}

// replaceVariables 替换模板中的变量
// 参数:
//
//	content: 模板内容
//	data: 模板数据
//
// 返回值:
//
//	string: 替换后的内容
func replaceVariables(content string, data *TemplateData) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	result := re.ReplaceAllStringFunc(content, func(match string) string {
		varName := strings.Trim(match, "${}")
		varName = strings.TrimSpace(varName)

		switch varName {
		case "execPath":
			return data.ExecPath
		case "programPID":
			return strconv.Itoa(data.ProgramPID)
		case "firstRunTime":
			return data.FirstRunTime
		case "lastRunTime":
			return data.LastRunTime
		case "runCount":
			return strconv.Itoa(data.RunCount)
		case "currentTime":
			return data.CurrentTime
		case "currentIPv4":
			return data.CurrentIPv4
		case "currentIPv6":
			return data.CurrentIPv6
		default:
			if strings.HasPrefix(varName, "lastRecord.") {
				field := strings.TrimPrefix(varName, "lastRecord.")
				if data.LastRecord != nil {
					switch field {
					case "LastRunTime":
						return data.LastRecord.LastRunTime
					case "IPv4":
						return data.LastRecord.IPv4
					case "IPv6":
						return data.LastRecord.IPv6
					case "EmailSent":
						return strconv.FormatBool(data.LastRecord.EmailSent)
					case "EmailResult":
						return data.LastRecord.EmailResult
					case "RunCount":
						return strconv.Itoa(data.LastRecord.RunCount)
					}
				}
			}
			return match
		}
	})

	return result
}

// parseTemplate 解析模板，处理条件判断
// 参数:
//
//	template: 模板内容
//	data: 模板数据
//
// 返回值:
//
//	string: 解析后的内容
func parseTemplate(template string, data *TemplateData) string {
	re := regexp.MustCompile(`\s*<if condition="([^"]+)">([\s\S]*?)</if>\s*`)

	result := re.ReplaceAllStringFunc(template, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		condition := submatches[1]
		content := submatches[2]

		if evaluateCondition(condition, data) {
			return replaceVariables(content, data)
		}

		return ""
	})

	return replaceVariables(result, data)
}

// renderMailTemplate 渲染邮件模板
// 参数:
//
//	templatePath: 模板文件路径
//	data: 模板数据
//
// 返回值:
//
//	string: 渲染后的内容
//	error: 错误信息
func renderMailTemplate(templatePath string, data *TemplateData) (string, error) {
	templateContent, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("读取模板文件失败: %v", err)
	}

	return parseTemplate(string(templateContent), data), nil
}
