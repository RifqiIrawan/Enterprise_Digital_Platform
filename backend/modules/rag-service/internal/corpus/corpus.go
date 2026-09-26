// Package corpus membaca berkas markdown dokumentasi platform dan
// memotongnya jadi bagian-bagian yang bisa dicari satu per satu.
//
// Pemotongan mengikuti HEADING, bukan jumlah karakter tetap. Alasannya: potongan
// sepanjang N karakter rutin memotong tabel di tengah baris dan memisahkan
// rumus dari kalimat yang menjelaskannya, lalu potongan itu muncul sebagai
// kutipan yang tidak bisa dibaca orang. Heading markdown sudah merupakan
// pembagian yang ditulis manusia untuk dibaca manusia; memakai batas itu
// membuat setiap kutipan punya judul yang jujur ("Rumus OEE"), yang juga jadi
// bagian paling berbobot saat dicari.
package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxChunkChars: satu bagian yang jauh lebih panjang dari ini dipecah lagi per
// paragraf, supaya satu heading raksasa tidak menghabiskan seluruh jatah
// konteks yang dikirim ke model. Angkanya kira-kira 500-700 token untuk teks
// Indonesia; empat potongan sebesar ini masih menyisakan banyak ruang di
// jendela konteks mana pun.
const maxChunkChars = 2400

// minChunkChars: potongan yang lebih pendek dari ini digabungkan ke potongan
// berikutnya. Heading yang isinya cuma satu kalimat pengantar bukan jawaban
// yang berguna untuk dikutip sendirian.
const minChunkChars = 120

type Chunk struct {
	Index     int
	Heading   string
	Content   string
	CharCount int
}

type Document struct {
	SourcePath  string
	Title       string
	ContentHash string
	ByteSize    int
	Chunks      []Chunk
}

func (d Document) ChunkCount() int { return len(d.Chunks) }

// Walk membaca seluruh berkas .md di dir (rekursif) dan mengembalikannya
// terurut per path, supaya hasil ingest berulang selalu sama urutannya.
// Berkas selain .md diabaikan: korpus ini sengaja hanya dokumentasi, bukan
// kode dan bukan catatan lepas.
func Walk(dir string) ([]Document, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	docs := make([]Document, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = filepath.Base(path)
		}
		docs = append(docs, Parse(filepath.ToSlash(rel), string(raw)))
	}
	return docs, nil
}

// Parse memotong satu berkas markdown. Judul dokumen diambil dari heading H1
// pertama kalau ada; kalau tidak, dari nama berkasnya.
func Parse(sourcePath, content string) Document {
	sum := sha256.Sum256([]byte(content))
	doc := Document{
		SourcePath:  sourcePath,
		Title:       deriveTitle(sourcePath, content),
		ContentHash: hex.EncodeToString(sum[:]),
		ByteSize:    len(content),
	}

	type section struct {
		heading string
		body    []string
	}
	sections := []section{{heading: ""}}
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		// Heading di DALAM blok kode (mis. komentar shell "# jalankan ini")
		// bukan heading. Tanpa pemeriksaan pagar ini, satu skrip di dokumentasi
		// bisa memecah dokumen jadi belasan potongan palsu.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence && isHeading(line) {
			sections = append(sections, section{heading: headingText(line)})
			continue
		}
		last := &sections[len(sections)-1]
		last.body = append(last.body, line)
	}

	index := 0
	var pending string
	var pendingHeading string
	flush := func(heading, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		doc.Chunks = append(doc.Chunks, Chunk{
			Index:     index,
			Heading:   heading,
			Content:   body,
			CharCount: len([]rune(body)),
		})
		index++
	}

	for _, s := range sections {
		body := strings.TrimSpace(strings.Join(s.body, "\n"))
		if body == "" {
			continue
		}
		// Bagian yang terlalu pendek ditahan dan digabung ke bagian
		// berikutnya, membawa heading-nya sendiri sebagai awalan supaya
		// konteksnya tidak hilang saat digabung.
		if len([]rune(body)) < minChunkChars {
			if pending != "" {
				pending += "\n\n"
			} else {
				pendingHeading = s.heading
			}
			pending += headingPrefix(s.heading) + body
			continue
		}
		if pending != "" {
			body = pending + "\n\n" + headingPrefix(s.heading) + body
			if pendingHeading != "" {
				s.heading = pendingHeading
			}
			pending, pendingHeading = "", ""
		}
		for _, part := range splitLong(body) {
			flush(s.heading, part)
		}
	}
	if pending != "" {
		flush(pendingHeading, pending)
	}
	return doc
}

func headingPrefix(heading string) string {
	if heading == "" {
		return ""
	}
	return heading + "\n"
}

// splitLong memecah bagian yang kelewat panjang di batas paragraf, bukan di
// tengah kalimat. Kalau satu paragraf sendirian sudah melebihi batas (tabel
// panjang, blok kode), paragraf itu dibiarkan utuh: memotongnya menghasilkan
// kutipan yang rusak, dan potongan kelewat besar cuma mahal, bukan salah.
func splitLong(body string) []string {
	if len([]rune(body)) <= maxChunkChars {
		return []string{body}
	}
	var out []string
	var cur strings.Builder
	for _, para := range strings.Split(body, "\n\n") {
		if cur.Len() > 0 && len([]rune(cur.String()))+len([]rune(para)) > maxChunkChars {
			out = append(out, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(para)
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, strings.TrimSpace(cur.String()))
	}
	return out
}

func isHeading(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	hashes := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	return hashes >= 1 && hashes <= 6 && strings.HasPrefix(strings.TrimLeft(trimmed, "#"), " ")
}

func headingText(line string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "# "))
}

func deriveTitle(sourcePath, content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			if t := headingText(line); t != "" {
				return t
			}
		}
	}
	base := filepath.Base(sourcePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
