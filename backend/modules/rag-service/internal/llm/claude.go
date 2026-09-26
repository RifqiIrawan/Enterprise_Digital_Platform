package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// DefaultModel: Claude Opus 5. Model dipilih lewat env (RAG_MODEL) supaya bisa
// diturunkan ke tier yang lebih murah kalau nanti volumenya besar -- tapi
// defaultnya BUKAN model termurah: jawaban yang salah tentang cara kerja
// sistem keuangan lebih mahal daripada selisih tokennya.
const DefaultModel = "claude-opus-5"

// maxTokens: jawaban chatbot dokumentasi pendek, tapi plafon yang terlalu
// rendah memotong jawaban di tengah kalimat dan memaksa orang bertanya ulang
// (yang justru membayar input token dua kali). 8000 jauh di atas kebutuhan
// normal dan tetap aman dari timeout HTTP untuk permintaan non-streaming.
const maxTokens = 8000

type ClaudeAnswerer struct {
	client anthropic.Client
	model  string
	effort string
}

// NewClaude mengembalikan nil kalau tidak ada kredensial. Itu bukan kegagalan:
// service tetap jalan, /ask tetap mengembalikan kutipan, dan status
// RETRIEVED_ONLY memberi tahu UI kenapa tidak ada jawaban naratif. Menjadikan
// ini fatal akan membuat seluruh pencarian dokumentasi ikut mati hanya karena
// belum ada kunci API.
func NewClaude(apiKey, model, effort string) *ClaudeAnswerer {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	if model == "" {
		model = DefaultModel
	}
	return &ClaudeAnswerer{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
		effort: effort,
	}
}

func (c *ClaudeAnswerer) Model() string { return c.model }

func (c *ClaudeAnswerer) Answer(ctx context.Context, question string, passages []Passage) (Result, error) {
	if len(passages) == 0 {
		return Result{}, errors.New("tidak ada kutipan untuk dijadikan dasar jawaban")
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
		// System di-cache: isinya sama persis untuk setiap pertanyaan, jadi
		// tidak ada alasan membayarnya sebagai input baru terus-menerus.
		// Kutipan dan pertanyaan sengaja TIDAK ikut di-cache -- keduanya
		// berbeda tiap permintaan, dan menandainya cacheable hanya akan
		// membuat cache meleset tanpa manfaat.
		System: []anthropic.TextBlockParam{{
			Text:         systemPrompt,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(BuildPrompt(question, passages))),
		},
		// Adaptive thinking: menimbang kutipan yang saling melengkapi (atau
		// bertentangan) bukan pekerjaan sepele, tapi juga bukan penalaran
		// berat -- effort rendah adalah titik yang tepat untuk jalur chat
		// bervolume tinggi seperti ini, dan bisa dinaikkan lewat RAG_EFFORT
		// tanpa menyentuh kode.
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
	}
	if c.effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(c.effort)}
	}

	resp, err := c.client.Messages.New(ctx, params)
	if err != nil {
		return Result{}, fmt.Errorf("panggil Claude: %w", err)
	}

	result := Result{
		Model:        c.model,
		InputTokens:  int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
	}

	// Penolakan datang sebagai HTTP 200 dengan stop_reason "refusal", bukan
	// sebagai galat -- membaca Content lebih dulu akan menampilkan jawaban
	// kosong seolah-olah modelnya tidak punya pendapat.
	if resp.StopReason == anthropic.StopReasonRefusal {
		result.Refused = true
		result.RefusalReason = string(resp.StopDetails.Category)
		return result, nil
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	result.Text = strings.TrimSpace(text.String())
	if result.Text == "" {
		return result, errors.New("Claude tidak mengembalikan teks jawaban")
	}
	return result, nil
}
