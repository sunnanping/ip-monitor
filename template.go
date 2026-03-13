package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"unicode"
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

// TokenType 定义token类型
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdentifier
	TokenNumber
	TokenString
	TokenOperator
	TokenLParen    // (
	TokenRParen    // )
	TokenLBracket  // [
	TokenRBracket  // ]
	TokenComma     // ,
	TokenDot       // .
)

// Token 词法单元
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// Lexer 词法分析器
type Lexer struct {
	input string
	pos   int
	tokens []Token
}

// NewLexer 创建新的词法分析器
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:  input,
		pos:    0,
		tokens: make([]Token, 0),
	}
}

// Tokenize 将输入字符串转换为token列表
func (l *Lexer) Tokenize() []Token {
	for l.pos < len(l.input) {
		l.skipWhitespace()
		if l.pos >= len(l.input) {
			break
		}

		ch := l.input[l.pos]

		switch ch {
		case '(':
			l.tokens = append(l.tokens, Token{Type: TokenLParen, Value: "(", Pos: l.pos})
			l.pos++
		case ')':
			l.tokens = append(l.tokens, Token{Type: TokenRParen, Value: ")", Pos: l.pos})
			l.pos++
		case '[':
			l.tokens = append(l.tokens, Token{Type: TokenLBracket, Value: "[", Pos: l.pos})
			l.pos++
		case ']':
			l.tokens = append(l.tokens, Token{Type: TokenRBracket, Value: "]", Pos: l.pos})
			l.pos++
		case ',':
			l.tokens = append(l.tokens, Token{Type: TokenComma, Value: ",", Pos: l.pos})
			l.pos++
		case '.':
			l.tokens = append(l.tokens, Token{Type: TokenDot, Value: ".", Pos: l.pos})
			l.pos++
		case '"', '\'':
			l.readString(ch)
		default:
			if unicode.IsLetter(rune(ch)) || ch == '_' || ch == '$' {
				l.readIdentifier()
			} else if unicode.IsDigit(rune(ch)) {
				l.readNumber()
			} else if l.isOperatorStart(ch) {
				l.readOperator()
			} else {
				l.pos++
			}
		}
	}

	l.tokens = append(l.tokens, Token{Type: TokenEOF, Value: "", Pos: l.pos})
	return l.tokens
}

// skipWhitespace 跳过空白字符
func (l *Lexer) skipWhitespace() {
	for l.pos < len(l.input) && (l.input[l.pos] == ' ' || l.input[l.pos] == '\t' || l.input[l.pos] == '\n' || l.input[l.pos] == '\r') {
		l.pos++
	}
}

// isOperatorStart 检查字符是否是操作符的开始
func (l *Lexer) isOperatorStart(ch byte) bool {
	operators := []string{"==", "!=", ">=", "<=", ">", "<", "&&", "||", "!", "+", "-", "*", "/", "%"}
	for _, op := range operators {
		if len(op) > 0 && op[0] == ch {
			return true
		}
	}
	return false
}

// readIdentifier 读取标识符
func (l *Lexer) readIdentifier() {
	start := l.pos
	for l.pos < len(l.input) && (unicode.IsLetter(rune(l.input[l.pos])) || unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '_' || l.input[l.pos] == '.' || l.input[l.pos] == '$') {
		l.pos++
	}
	value := l.input[start:l.pos]
	l.tokens = append(l.tokens, Token{Type: TokenIdentifier, Value: value, Pos: start})
}

// readNumber 读取数字
func (l *Lexer) readNumber() {
	start := l.pos
	for l.pos < len(l.input) && (unicode.IsDigit(rune(l.input[l.pos])) || l.input[l.pos] == '.') {
		l.pos++
	}
	value := l.input[start:l.pos]
	l.tokens = append(l.tokens, Token{Type: TokenNumber, Value: value, Pos: start})
}

// readString 读取字符串
func (l *Lexer) readString(quote byte) {
	start := l.pos
	l.pos++ // 跳过开始的引号
	for l.pos < len(l.input) && l.input[l.pos] != quote {
		if l.input[l.pos] == '\\' && l.pos+1 < len(l.input) {
			l.pos += 2
		} else {
			l.pos++
		}
	}
	if l.pos < len(l.input) {
		l.pos++ // 跳过结束的引号
	}
	value := l.input[start:l.pos]
	l.tokens = append(l.tokens, Token{Type: TokenString, Value: value, Pos: start})
}

// readOperator 读取操作符
func (l *Lexer) readOperator() {
	start := l.pos
	// 尝试读取双字符操作符
	if l.pos+1 < len(l.input) {
		twoChar := l.input[l.pos : l.pos+2]
		if twoChar == "==" || twoChar == "!=" || twoChar == ">=" || twoChar == "<=" || twoChar == "&&" || twoChar == "||" {
			l.pos += 2
			l.tokens = append(l.tokens, Token{Type: TokenOperator, Value: twoChar, Pos: start})
			return
		}
	}
	// 单字符操作符
	l.pos++
	l.tokens = append(l.tokens, Token{Type: TokenOperator, Value: string(l.input[start]), Pos: start})
}

// NodeType AST节点类型
type NodeType int

const (
	NodeBinary NodeType = iota
	NodeUnary
	NodeLiteral
	NodeIdentifier
	NodeMemberAccess
	NodeMethodCall
	NodeArrayAccess
)

// ASTNode 抽象语法树节点
type ASTNode struct {
	Type     NodeType
	Value    string
	Left     *ASTNode
	Right    *ASTNode
	Children []*ASTNode
}

// Parser 语法分析器
type Parser struct {
	tokens []Token
	pos    int
}

// NewParser 创建新的语法分析器
func NewParser(tokens []Token) *Parser {
	return &Parser{
		tokens: tokens,
		pos:    0,
	}
}

// Parse 解析表达式，返回AST根节点
func (p *Parser) Parse() *ASTNode {
	return p.parseOr()
}

// parseOr 解析逻辑或表达式 (优先级最低)
func (p *Parser) parseOr() *ASTNode {
	left := p.parseAnd()

	for p.matchOperator("||") || p.matchKeyword("or") {
		op := p.previous().Value
		right := p.parseAnd()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseAnd 解析逻辑与表达式
func (p *Parser) parseAnd() *ASTNode {
	left := p.parseEquality()

	for p.matchOperator("&&") || p.matchKeyword("and") {
		op := p.previous().Value
		right := p.parseEquality()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseEquality 解析相等性表达式 (==, !=)
func (p *Parser) parseEquality() *ASTNode {
	left := p.parseComparison()

	for p.matchOperator("==") || p.matchOperator("!=") {
		op := p.previous().Value
		right := p.parseComparison()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseComparison 解析比较表达式 (<, >, <=, >=)
func (p *Parser) parseComparison() *ASTNode {
	left := p.parseAdditive()

	for p.matchOperator("<") || p.matchOperator(">") || p.matchOperator("<=") || p.matchOperator(">=") {
		op := p.previous().Value
		right := p.parseAdditive()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseAdditive 解析加减表达式 (+, -)
func (p *Parser) parseAdditive() *ASTNode {
	left := p.parseMultiplicative()

	for p.matchOperator("+") || p.matchOperator("-") {
		op := p.previous().Value
		right := p.parseMultiplicative()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseMultiplicative 解析乘除表达式 (*, /, %)
func (p *Parser) parseMultiplicative() *ASTNode {
	left := p.parseUnary()

	for p.matchOperator("*") || p.matchOperator("/") || p.matchOperator("%") {
		op := p.previous().Value
		right := p.parseUnary()
		left = &ASTNode{
			Type:  NodeBinary,
			Value: op,
			Left:  left,
			Right: right,
		}
	}

	return left
}

// parseUnary 解析一元表达式 (!, not, -)
func (p *Parser) parseUnary() *ASTNode {
	if p.matchOperator("!") || p.matchKeyword("not") {
		op := p.previous().Value
		operand := p.parseUnary()
		return &ASTNode{
			Type:  NodeUnary,
			Value: op,
			Left:  operand,
		}
	}

	if p.matchOperator("-") {
		op := p.previous().Value
		operand := p.parseUnary()
		return &ASTNode{
			Type:  NodeUnary,
			Value: op,
			Left:  operand,
		}
	}

	return p.parsePrimary()
}

// parsePrimary 解析基本表达式
func (p *Parser) parsePrimary() *ASTNode {
	// 解析括号表达式
	if p.match(TokenLParen) {
		expr := p.Parse()
		p.consume(TokenRParen, "期望 ')'")
		return expr
	}

	// 解析数字
	if p.match(TokenNumber) {
		return &ASTNode{
			Type:  NodeLiteral,
			Value: p.previous().Value,
		}
	}

	// 解析字符串
	if p.match(TokenString) {
		return &ASTNode{
			Type:  NodeLiteral,
			Value: p.previous().Value,
		}
	}

	// 解析标识符（可能包含成员访问、方法调用等）
	if p.match(TokenIdentifier) {
		return p.parsePostfix(&ASTNode{
			Type:  NodeIdentifier,
			Value: p.previous().Value,
		})
	}

	return nil
}

// parsePostfix 解析后缀表达式（成员访问、方法调用、数组访问）
func (p *Parser) parsePostfix(node *ASTNode) *ASTNode {
	for {
		// 成员访问: obj.property
		if p.match(TokenDot) {
			if p.match(TokenIdentifier) {
				member := p.previous().Value
				node = &ASTNode{
					Type:     NodeMemberAccess,
					Value:    member,
					Left:     node,
					Children: []*ASTNode{node},
				}
			}
		} else if p.match(TokenLBracket) {
			// 数组访问: array[index]
			index := p.Parse()
			p.consume(TokenRBracket, "期望 ']'")
			node = &ASTNode{
				Type:     NodeArrayAccess,
				Value:    "[]",
				Left:     node,
				Right:    index,
				Children: []*ASTNode{node, index},
			}
		} else {
			break
		}
	}
	return node
}

// match 匹配指定类型的token
func (p *Parser) match(tokenType TokenType) bool {
	if p.check(tokenType) {
		p.advance()
		return true
	}
	return false
}

// matchOperator 匹配操作符
func (p *Parser) matchOperator(op string) bool {
	if p.check(TokenOperator) && p.peek().Value == op {
		p.advance()
		return true
	}
	return false
}

// matchKeyword 匹配关键字（and, or, not等）
func (p *Parser) matchKeyword(keyword string) bool {
	if p.check(TokenIdentifier) && strings.ToLower(p.peek().Value) == keyword {
		p.advance()
		return true
	}
	return false
}

// check 检查当前token是否是指定类型
func (p *Parser) check(tokenType TokenType) bool {
	if p.isAtEnd() {
		return false
	}
	return p.peek().Type == tokenType
}

// advance 前进到下一个token
func (p *Parser) advance() Token {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.previous()
}

// isAtEnd 检查是否到达token列表末尾
func (p *Parser) isAtEnd() bool {
	return p.peek().Type == TokenEOF
}

// peek 查看当前token
func (p *Parser) peek() Token {
	return p.tokens[p.pos]
}

// previous 获取前一个token
func (p *Parser) previous() Token {
	return p.tokens[p.pos-1]
}

// consume 消费指定类型的token，否则报错
func (p *Parser) consume(tokenType TokenType, message string) Token {
	if p.check(tokenType) {
		return p.advance()
	}
	panic(fmt.Sprintf("%s, 实际得到: %v", message, p.peek()))
}

// Evaluator 表达式求值器
type Evaluator struct {
	data *TemplateData
}

// NewEvaluator 创建新的求值器
func NewEvaluator(data *TemplateData) *Evaluator {
	return &Evaluator{data: data}
}

// Evaluate 评估AST节点
func (e *Evaluator) Evaluate(node *ASTNode) interface{} {
	if node == nil {
		return nil
	}

	switch node.Type {
	case NodeBinary:
		return e.evaluateBinary(node)
	case NodeUnary:
		return e.evaluateUnary(node)
	case NodeLiteral:
		return e.evaluateLiteral(node)
	case NodeIdentifier:
		return e.evaluateIdentifier(node)
	case NodeMemberAccess:
		return e.evaluateMemberAccess(node)
	default:
		return nil
	}
}

// evaluateBinary 评估二元表达式
func (e *Evaluator) evaluateBinary(node *ASTNode) interface{} {
	left := e.Evaluate(node.Left)
	right := e.Evaluate(node.Right)

	switch node.Value {
	case "||", "or":
		return toBool(left) || toBool(right)
	case "&&", "and":
		return toBool(left) && toBool(right)
	case "==":
		return compareEqual(left, right)
	case "!=":
		return !compareEqual(left, right)
	case "<":
		return compareLess(left, right)
	case ">":
		return compareGreater(left, right)
	case "<=":
		return compareLessEqual(left, right)
	case ">=":
		return compareGreaterEqual(left, right)
	case "+":
		return add(left, right)
	case "-":
		return subtract(left, right)
	case "*":
		return multiply(left, right)
	case "/":
		return divide(left, right)
	case "%":
		return modulo(left, right)
	default:
		return nil
	}
}

// evaluateUnary 评估一元表达式
func (e *Evaluator) evaluateUnary(node *ASTNode) interface{} {
	operand := e.Evaluate(node.Left)

	switch node.Value {
	case "!", "not":
		return !toBool(operand)
	case "-":
		if num, ok := toFloat64(operand); ok {
			return -num
		}
		return nil
	default:
		return operand
	}
}

// evaluateLiteral 评估字面量
func (e *Evaluator) evaluateLiteral(node *ASTNode) interface{} {
	value := node.Value

	// 尝试解析为数字
	if num, err := strconv.ParseFloat(value, 64); err == nil {
		return num
	}

	// 处理字符串字面量（移除引号）
	if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) ||
		(strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
		return value[1 : len(value)-1]
	}

	// 处理布尔值
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// 处理nil
	if value == "nil" || value == "null" {
		return nil
	}

	return value
}

// evaluateIdentifier 评估标识符
func (e *Evaluator) evaluateIdentifier(node *ASTNode) interface{} {
	name := node.Value

	switch name {
	case "RunCount":
		return float64(e.data.RunCount)
	case "sendMsg", "SendMsg":
		return e.data.SendMsg
	case "lastRecord":
		return e.data.LastRecord
	case "currentTime":
		return e.data.CurrentTime
	case "currentIPv4":
		return e.data.CurrentIPv4
	case "currentIPv6":
		return e.data.CurrentIPv6
	case "execPath":
		return e.data.ExecPath
	case "programPID":
		return float64(e.data.ProgramPID)
	case "firstRunTime":
		return e.data.FirstRunTime
	case "lastRunTime":
		return e.data.LastRunTime
	default:
		return name
	}
}

// evaluateMemberAccess 评估成员访问
func (e *Evaluator) evaluateMemberAccess(node *ASTNode) interface{} {
	base := e.Evaluate(node.Children[0])
	member := node.Value

	if record, ok := base.(*RunRecord); ok && record != nil {
		switch member {
		case "LastRunTime":
			return record.LastRunTime
		case "IPv4":
			return record.IPv4
		case "IPv6":
			return record.IPv6
		case "EmailSent":
			return record.EmailSent
		case "EmailResult":
			return record.EmailResult
		case "RunCount":
			return float64(record.RunCount)
		default:
			return nil
		}
	}

	return nil
}

// toBool 将值转换为布尔值
func toBool(v interface{}) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	if f, ok := toFloat64(v); ok {
		return f != 0
	}
	if s, ok := v.(string); ok {
		return s != "" && s != "false" && s != "0"
	}
	return true
}

// toFloat64 将值转换为float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// toString 将值转换为字符串
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := toFloat64(v); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	if b, ok := v.(bool); ok {
		return strconv.FormatBool(b)
	}
	return fmt.Sprintf("%v", v)
}

// compareEqual 比较相等
func compareEqual(left, right interface{}) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}

	// 数字比较
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l == r
		}
	}

	// 字符串比较
	return toString(left) == toString(right)
}

// compareLess 比较小于
func compareLess(left, right interface{}) bool {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l < r
		}
	}
	return toString(left) < toString(right)
}

// compareGreater 比较大于
func compareGreater(left, right interface{}) bool {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l > r
		}
	}
	return toString(left) > toString(right)
}

// compareLessEqual 比较小于等于
func compareLessEqual(left, right interface{}) bool {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l <= r
		}
	}
	return toString(left) <= toString(right)
}

// compareGreaterEqual 比较大于等于
func compareGreaterEqual(left, right interface{}) bool {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l >= r
		}
	}
	return toString(left) >= toString(right)
}

// add 加法运算
func add(left, right interface{}) interface{} {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l + r
		}
	}
	return toString(left) + toString(right)
}

// subtract 减法运算
func subtract(left, right interface{}) interface{} {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l - r
		}
	}
	return nil
}

// multiply 乘法运算
func multiply(left, right interface{}) interface{} {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok {
			return l * r
		}
	}
	return nil
}

// divide 除法运算
func divide(left, right interface{}) interface{} {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok && r != 0 {
			return l / r
		}
	}
	return nil
}

// modulo 取模运算
func modulo(left, right interface{}) interface{} {
	if l, ok := toFloat64(left); ok {
		if r, ok := toFloat64(right); ok && r != 0 {
			return float64(int64(l) % int64(r))
		}
	}
	return nil
}

// evaluateCondition 评估条件表达式（新实现）
func evaluateCondition(condition string, data *TemplateData) (bool, []string, error) {
	// 词法分析
	lexer := NewLexer(condition)
	tokens := lexer.Tokenize()

	// 语法分析
	parser := NewParser(tokens)
	ast := parser.Parse()

	// 求值
	evaluator := NewEvaluator(data)
	result := evaluator.Evaluate(ast)

	return toBool(result), nil, nil
}

// replaceVariables 替换模板中的变量
func replaceVariables(content string, data *TemplateData) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	result := re.ReplaceAllStringFunc(content, func(match string) string {
		varName := strings.Trim(match, "${}")
		varName = strings.TrimSpace(varName)

		// 使用新的表达式求值器来解析变量
		lexer := NewLexer(varName)
		tokens := lexer.Tokenize()
		parser := NewParser(tokens)
		ast := parser.Parse()
		evaluator := NewEvaluator(data)
		value := evaluator.Evaluate(ast)

		if value == nil {
			return "nil"
		}
		return toString(value)
	})

	return result
}

// parseTemplate 解析模板，处理条件判断
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

		// 评估条件
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
func renderMailTemplate(templatePath string, data *TemplateData) (string, []string, error) {
	templateContent, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return "", nil, fmt.Errorf("读取模板文件失败: %v", err)
	}

	return parseTemplate(string(templateContent), data)
}
