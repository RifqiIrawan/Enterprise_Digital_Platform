// Package llm memisahkan "menyusun jawaban" dari "mencari bahan". Yang
// dipanggil httpapi hanyalah antarmuka Answerer; implementasi Claude ada di
// claude.go, dan test memakai stub tanpa jaringan.
//
// Pemisahan ini bukan kemewahan: ini satu-satunya alasan service ini tetap
// berguna tanpa kunci API. Kalau Answerer nil, /ask tetap mengembalikan
// kutipan yang ditemukan beserta status RETRIEVED_ONLY -- bukan galat, dan
// terutama bukan jawaban karangan.
package llm

import (
	"context"
	"fmt"
	"strings"
)

// Passage adalah satu potongan dokumentasi yang dikirim ke model sebagai
// bahan. Ref adalah nomor kutipan ([1], [2], ...) yang dipakai di jawaban --
// nomor itu ditentukan di sini, bukan oleh model, supaya nomor di teks selalu
// cocok dengan daftar sumber yang ditampilkan di UI.
type Passage struct {
	Ref        int    `json:"ref"`
	SourcePath string `json:"source_path"`
	Title      string `json:"title"`
	Heading    string `json:"heading"`
	Content    string `json:"content"`
}

type Result struct {
	Text          string
	Model         string
	InputTokens   int
	OutputTokens  int
	Refused       bool
	RefusalReason string
}

type Answerer interface {
	// Answer menyusun jawaban HANYA dari passages. Implementasi wajib
	// mengembalikan galat, bukan jawaban kosong, kalau panggilannya gagal --
	// pemanggil membedakan keduanya saat mencatat rag_queries.
	Answer(ctx context.Context, question string, passages []Passage) (Result, error)
	Model() string
}

// systemPrompt sengaja pendek dan hanya berisi aturan yang benar-benar
// mengubah perilaku. Tiga aturan pertamanya adalah alasan service ini boleh
// dipercaya: jawab dari kutipan, akui kalau tidak ada, dan sebutkan nomor
// sumbernya. Aturan bahasa ada karena seluruh dokumentasi dan UI platform ini
// berbahasa Indonesia; jawaban berbahasa Inggris akan terasa seperti produk
// lain.
const systemPrompt = `Kamu asisten dokumentasi Enterprise Digital Platform (EDP), sebuah platform ERP.

Aturan:
1. Jawab HANYA berdasarkan kutipan dokumentasi yang diberikan. Jangan memakai pengetahuan umum tentang ERP atau menebak nama endpoint, tabel, atau angka yang tidak tertulis di kutipan.
2. Kalau kutipannya tidak memuat jawaban, katakan begitu dengan jelas dan sebutkan bagian mana yang paling mendekati. Jangan mengarang.
3. Sebutkan nomor sumber dalam kurung siku setiap kali kamu memakai sebuah kutipan, misalnya [1] atau [2][3].
4. Jawab dalam bahasa Indonesia, langsung ke pokoknya, maksimal beberapa paragraf pendek. Pakai daftar kalau isinya memang daftar.
5. Kalau kutipan saling bertentangan, sebutkan pertentangannya alih-alih memilih salah satu diam-diam.`

// BuildPrompt menyusun pesan pengguna: kutipan dulu, pertanyaan terakhir.
// Urutan itu disengaja -- bagian yang panjang dan berulang di depan, yang
// berubah tiap permintaan di belakang, supaya prompt caching punya peluang
// mengenali awalan yang sama (lihat catatan di claude.go).
func BuildPrompt(question string, passages []Passage) string {
	var b strings.Builder
	b.WriteString("Kutipan dokumentasi:\n\n")
	for _, p := range passages {
		fmt.Fprintf(&b, "[%d] %s", p.Ref, p.Title)
		if p.Heading != "" {
			fmt.Fprintf(&b, " — %s", p.Heading)
		}
		fmt.Fprintf(&b, " (%s)\n%s\n\n", p.SourcePath, p.Content)
	}
	b.WriteString("Pertanyaan: ")
	b.WriteString(question)
	return b.String()
}
