package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
)

type TemplateData struct {
	ExecPath     string
	ProgramPID   int
	FirstRunTime string
	LastRunTime  string
	RunCount     int
	SendMsg      bool
	LastRecord   *RunRecord
	CurrentTime  string
	CurrentIPv4  string
	CurrentIPv6  string
}

func evaluateCondition(condition string, data *TemplateData) bool {
	condition = strings.TrimSpace(condition)

	if condition == "lastRecord == nil" {
		return data.LastRecord == nil
	}

	if condition == "RunCount > 2" {
		return data.RunCount > 2
	}

	if condition == "RunCount > 2 and sendMsg == true" {
		return data.RunCount > 2 && data.SendMsg
	}

	return false
}

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

func parseTemplate(template string, data *TemplateData) string {
	re := regexp.MustCompile(`<if condition="([^"]+)">([\s\S]*?)</if>`)

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

func renderMailTemplate(templatePath string, data *TemplateData) (string, error) {
	templateContent, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("读取模板文件失败: %v", err)
	}

	return parseTemplate(string(templateContent), data), nil
}
