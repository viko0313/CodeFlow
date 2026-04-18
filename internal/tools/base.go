package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type CalculatorTool struct{}

func NewCalculatorTool() *CalculatorTool { return &CalculatorTool{} }

func (t *CalculatorTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "calculator",
		Desc: "A simple calculator for mathematical expressions. Input should be a valid math expression like '3 * 5' or '100 / 4'.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"expression": {
				Type:     schema.String,
				Desc:     "The mathematical expression to evaluate",
				Required: true,
			},
		}),
	}, nil
}

func (t *CalculatorTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	var args struct {
		Expression string `json:"expression"`
	}
	if err := json.Unmarshal([]byte(argsInJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	result, err := evalArithmetic(args.Expression)
	if err != nil {
		return "", fmt.Errorf("calculation error: %w", err)
	}
	return fmt.Sprintf("Expression %q = %s", args.Expression, strconv.FormatFloat(result, 'f', -1, 64)), nil
}

type TimeTool struct{}

func NewTimeTool() *TimeTool { return &TimeTool{} }

func (t *TimeTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_current_time",
		Desc: "Get the current system time and date. Use this when user asks 'what time is it', 'what day is today', etc.",
	}, nil
}

func (t *TimeTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	now := time.Now()
	return now.Format("2006-01-02 15:04:05"), nil
}

type ModelInfoTool struct {
	Provider, Model string
}

func NewModelInfoTool(provider, model string) *ModelInfoTool {
	return &ModelInfoTool{Provider: provider, Model: model}
}

func (t *ModelInfoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_system_model_info",
		Desc: "Get the current LLM model and provider information being used by CyberClaw.",
	}, nil
}

func (t *ModelInfoTool) InvokableRun(ctx context.Context, argsInJSON string, opts ...tool.Option) (string, error) {
	return fmt.Sprintf("Provider: %s, Model: %s", t.Provider, t.Model), nil
}

type arithmeticParser struct {
	input string
	pos   int
}

func evalArithmetic(input string) (float64, error) {
	p := &arithmeticParser{input: strings.TrimSpace(input)}
	if p.input == "" {
		return 0, fmt.Errorf("empty expression")
	}
	value, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected character %q", p.input[p.pos])
	}
	return value, nil
}

func (p *arithmeticParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.match('+') {
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left += right
			continue
		}
		if p.match('-') {
			right, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			left -= right
			continue
		}
		return left, nil
	}
}

func (p *arithmeticParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.match('*') {
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			left *= right
			continue
		}
		if p.match('/') {
			right, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
			continue
		}
		return left, nil
	}
}

func (p *arithmeticParser) parseFactor() (float64, error) {
	p.skipSpaces()
	if p.match('+') {
		return p.parseFactor()
	}
	if p.match('-') {
		v, err := p.parseFactor()
		return -v, err
	}
	if p.match('(') {
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if !p.match(')') {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		return v, nil
	}
	return p.parseNumber()
}

func (p *arithmeticParser) parseNumber() (float64, error) {
	p.skipSpaces()
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if (ch >= '0' && ch <= '9') || ch == '.' {
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return 0, fmt.Errorf("expected number")
	}
	return strconv.ParseFloat(p.input[start:p.pos], 64)
}

func (p *arithmeticParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
}

func (p *arithmeticParser) match(ch byte) bool {
	if p.pos < len(p.input) && p.input[p.pos] == ch {
		p.pos++
		return true
	}
	return false
}
