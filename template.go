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

// validateVariables 验证条件表达式中的变量是否存在
// 参数:
//
//	expression: 表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	[]string: 未定义的变量列表
//	error: 解析错误
func validateVariables(expression string, data *TemplateData) ([]string, error) {
	// 处理括号
	for strings.Contains(expression, "(") {
		// 找到最内层的括号
		start := strings.LastIndex(expression, "(")
		end := strings.Index(expression[start:], ")") + start
		if start == -1 || end == -1 {
			return nil, fmt.Errorf("括号不匹配: %s", expression)
		}

		// 替换括号为占位符
		expression = expression[:start] + "valid" + expression[end+1:]
	}

	// 处理OR操作符
	if strings.Contains(expression, " or ") {
		// 分割OR操作符
		parts := strings.Split(expression, " or ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			undefined, err := validateSingleExpression(part, data)
			if err != nil {
				return undefined, err
			}
			if len(undefined) > 0 {
				return undefined, nil
			}
		}
		return nil, nil
	}

	// 处理AND操作符
	if strings.Contains(expression, " and ") {
		// 分割AND操作符
		parts := strings.Split(expression, " and ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			undefined, err := validateSingleExpression(part, data)
			if err != nil {
				return undefined, err
			}
			if len(undefined) > 0 {
				return undefined, nil
			}
		}
		return nil, nil
	}

	// 处理单个表达式
	return validateSingleExpression(expression, data)
}

// validateSingleExpression 验证单个表达式（不含逻辑操作符）
// 参数:
//
//	expression: 单个表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	[]string: 未定义的变量列表
//	error: 解析错误
func validateSingleExpression(expression string, data *TemplateData) ([]string, error) {
	// 移除空格以简化解析
	expression = strings.ReplaceAll(expression, " ", "")

	// 处理比较操作
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if strings.Contains(expression, op) {
			parts := strings.Split(expression, op)
			if len(parts) != 2 {
				return nil, fmt.Errorf("比较操作格式错误: %s", expression)
			}

			left := parts[0]
			right := parts[1]

			// 验证左侧变量
			if !isValidVariableWithDot(left) {
				return []string{left}, nil
			}

			// 右侧如果是数字、布尔值或nil，不需要验证
			if !isNumber(right) && !isBoolean(right) && right != "nil" {
				if !isValidVariableWithDot(right) {
					return []string{right}, nil
				}
			}

			return nil, nil
		}
	}

	// 处理布尔值
	if expression == "true" || expression == "false" {
		return nil, nil
	}

	// 处理变量
	if expression == "lastRecord" || expression == "lastRecord==nil" {
		return nil, nil
	}

	// 检查是否是有效的变量
	if isValidVariableWithDot(expression) {
		return nil, nil
	}

	// 未知变量
	return []string{expression}, nil
}

// isValidVariable 检查变量是否有效
// 参数:
//
//	varName: 变量名
//
// 返回值:
//
//	bool: 变量是否有效
func isValidVariable(varName string) bool {
	// 有效的变量名列表
	validVariables := map[string]bool{
		"RunCount":   true,
		"sendMsg":    true,
		"lastRecord": true,
	}

	return validVariables[varName]
}

// isNumber 检查字符串是否为数字
// 参数:
//
//	s: 字符串
//
// 返回值:
//
//	bool: 是否为数字
func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// isBoolean 检查字符串是否为布尔值
// 参数:
//
//	s: 字符串
//
// 返回值:
//
//	bool: 是否为布尔值
func isBoolean(s string) bool {
	return s == "true" || s == "false"
}

// splitByOperator 安全地分割操作符，避免分割变量名中的子串
// 参数:
//
//	s: 字符串
//	op: 操作符
//
// 返回值:
//
//	[]string: 分割后的字符串数组
func splitByOperator(s, op string) []string {
	var parts []string
	var current strings.Builder

	for i := 0; i < len(s); i++ {
		if i+len(op) <= len(s) && s[i:i+len(op)] == op {
			// 检查是否是独立的操作符
			isOperator := true

			// 检查前一个字符是否是字母或数字
			if i > 0 && (s[i-1] >= 'a' && s[i-1] <= 'z' || s[i-1] >= 'A' && s[i-1] <= 'Z' || s[i-1] >= '0' && s[i-1] <= '9' || s[i-1] == '.') {
				isOperator = false
			}

			// 检查后一个字符是否是字母或数字
			if i+len(op) < len(s) && (s[i+len(op)] >= 'a' && s[i+len(op)] <= 'z' || s[i+len(op)] >= 'A' && s[i+len(op)] <= 'Z' || s[i+len(op)] >= '0' && s[i+len(op)] <= '9' || s[i+len(op)] == '.') {
				isOperator = false
			}

			if isOperator {
				parts = append(parts, current.String())
				current.Reset()
				i += len(op) - 1
				continue
			}
		}
		current.WriteByte(s[i])
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// isValidVariableWithDot 检查带点的变量是否有效
// 参数:
//
//	varName: 变量名，支持带点的结构体成员变量
//
// 返回值:
//
//	bool: 变量是否有效
func isValidVariableWithDot(varName string) bool {
	// 有效的变量名列表
	validVariables := map[string]bool{
		"RunCount":    true,
		"sendMsg":     true,
		"lastRecord":  true,
		"currentTime": true,
		"currentIPv4": true,
		"currentIPv6": true,
	}

	// 检查是否是简单变量
	if validVariables[varName] {
		return true
	}

	// 检查是否是带点的变量，如lastRecord.LastRunTime
	parts := strings.Split(varName, ".")
	if len(parts) > 1 {
		// 检查基础变量是否有效
		baseVar := parts[0]
		if validVariables[baseVar] {
			// 对于lastRecord，检查其成员变量
			if baseVar == "lastRecord" {
				validMembers := map[string]bool{
					"LastRunTime": true,
					"IPv4":        true,
					"IPv6":        true,
					"EmailSent":   true,
					"EmailResult": true,
					"RunCount":    true,
				}
				member := parts[1]
				return validMembers[member]
			}
			return true
		}
	}

	return false
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
//	[]string: 未定义的变量列表
//	error: 解析错误
func evaluateCondition(condition string, data *TemplateData) (bool, []string, error) {
	condition = strings.TrimSpace(condition)

	// 验证变量
	undefined, err := validateVariables(condition, data)
	if err != nil {
		return false, undefined, err
	}
	if len(undefined) > 0 {
		return false, undefined, nil
	}

	// 评估表达式
	result := evaluateExpression(condition, data)
	return result, nil, nil
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
	if strings.Contains(expression, " or ") {
		parts := strings.Split(expression, " or ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if evaluateSingleExpressionForEval(part, data) {
				return true
			}
		}
		return false
	}

	// 处理AND操作符
	if strings.Contains(expression, " and ") {
		parts := strings.Split(expression, " and ")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if !evaluateSingleExpressionForEval(part, data) {
				return false
			}
		}
		return true
	}

	// 处理单个表达式
	return evaluateSingleExpressionForEval(expression, data)
}

// evaluateSingleExpressionForEval 评估单个表达式（不含逻辑操作符）
// 参数:
//
//	expression: 单个表达式字符串
//	data: 模板数据
//
// 返回值:
//
//	bool: 表达式是否为真
func evaluateSingleExpressionForEval(expression string, data *TemplateData) bool {
	// 移除空格以简化解析
	expression = strings.ReplaceAll(expression, " ", "")

	// 处理比较操作
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if strings.Contains(expression, op) {
			parts := strings.Split(expression, op)
			if len(parts) != 2 {
				return false
			}

			left := parts[0]
			right := parts[1]

			return evaluateCompare(left, right, op, data)
		}
	}

	// 处理布尔值
	if expression == "true" {
		return true
	}
	if expression == "false" {
		return false
	}

	// 处理变量
	if expression == "lastRecord" {
		return data.LastRecord != nil
	}

	return false
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
//	[]string: 未定义的变量列表
//	error: 解析错误
func parseTemplate(template string, data *TemplateData) (string, []string, error) {
	re := regexp.MustCompile(`\s*<if condition="([^"]+)">([\s\S]*?)</if>\s*`)

	var undefinedVariables []string
	var parseError error

	result := re.ReplaceAllStringFunc(template, func(match string) string {
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 3 {
			return match
		}

		condition := submatches[1]
		content := submatches[2]

		// 评估条件并验证变量
		conditionResult, undefined, err := evaluateCondition(condition, data)
		if err != nil {
			parseError = err
			return fmt.Sprintf("<!-- 模板错误: %v -->", err)
		}
		if len(undefined) > 0 {
			undefinedVariables = append(undefinedVariables, undefined...)
			return fmt.Sprintf("<!-- 未定义变量: %v -->", undefined)
		}

		if conditionResult {
			return replaceVariables(content, data)
		}

		return ""
	})

	// 替换剩余的变量
	result = replaceVariables(result, data)

	return result, undefinedVariables, parseError
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
//	[]string: 未定义的变量列表
//	error: 错误信息
func renderMailTemplate(templatePath string, data *TemplateData) (string, []string, error) {
	templateContent, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return "", nil, fmt.Errorf("读取模板文件失败: %v", err)
	}

	return parseTemplate(string(templateContent), data)
}
