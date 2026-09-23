package approvals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
)

// 固定模型版本；门槛是保守的转人工策略，不是安全正确率保证。
const jevModel = "jev-1.13.0"
const jevAllowConfidence = 0.5

func (s *Service) reviewJev(ctx context.Context, state json.RawMessage) (reviewResult, error) {
	body, err := json.Marshal(map[string]any{
		"model": jevModel, "state": state,
		"questions": map[string]any{"approval": map[string]any{
			"type": "choice", "instructions": reviewRules,
			"criteria": map[string]string{
				"allow": "信息充分，实际操作和额外权限符合用户授权，没有未授权的危险副作用。",
				"deny":  "有明确证据表明违反用户限制，或存在未授权的破坏、敏感数据外传等危险行为。",
				"ask":   "缺少必要证据，无法确认实际影响或用户授权，需人工审批。",
			},
		}},
	})
	if err != nil {
		return reviewResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.typesafe.ai/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return reviewResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.jevKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return reviewResult{}, fmt.Errorf("Jev 审核请求失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return reviewResult{}, fmt.Errorf("Jev HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Answers map[string]struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Confidence    *float64           `json:"confidence"`
			Probabilities map[string]float64 `json:"probabilities"`
		} `json:"answers"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&payload)
	if err != nil {
		return reviewResult{}, fmt.Errorf("Jev 响应无法解析")
	}
	answer := payload.Answers["approval"]
	if answer.Type != "choice" || answer.Confidence == nil || *answer.Confidence < 0 || *answer.Confidence > 1 {
		return reviewResult{}, fmt.Errorf("Jev 响应缺少合法置信度")
	}
	total := 0.0
	for _, option := range []string{"allow", "deny", "ask"} {
		p, ok := answer.Probabilities[option]
		if !ok || p < 0 || p > 1 {
			return reviewResult{}, fmt.Errorf("Jev 概率分布无效")
		}
		total += p
		if p > answer.Probabilities[answer.Choice] {
			return reviewResult{}, fmt.Errorf("Jev 决定与概率不符")
		}
	}
	if len(answer.Probabilities) != 3 || math.Abs(total-1) > 0.01 {
		return reviewResult{}, fmt.Errorf("Jev 概率分布无效")
	}
	switch answer.Choice {
	case "allow":
		if *answer.Confidence < jevAllowConfidence {
			return reviewResult{"ask", fmt.Sprintf("Jev 批准置信度 %.2f 低于门槛 %.2f，请人工确认", *answer.Confidence, jevAllowConfidence)}, nil
		}
		return reviewResult{"allow", "Jev 判断符合用户授权"}, nil
	case "deny":
		return reviewResult{"deny", "Jev 判断不符合审核规则"}, nil
	case "ask":
		return reviewResult{"ask", "Jev 无法确认操作影响或用户授权，请人工确认"}, nil
	default:
		return reviewResult{}, fmt.Errorf("Jev 返回未知决定")
	}
}
