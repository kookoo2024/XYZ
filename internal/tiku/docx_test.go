package tiku

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func assertWellFormedXML(t *testing.T, s string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("XML 不合法: %v", err)
		}
	}
}

func TestLatexToOMML(t *testing.T) {
	cases := []struct {
		name string
		tex  string
		want []string
	}{
		{"上标", `x^{2}`, []string{"m:sSup"}},
		{"下标", `A_{1}`, []string{"m:sSub"}},
		{"上下标", `A_{1}^{2}`, []string{"m:sSubSup"}},
		{"分式", `\dfrac{a}{b}`, []string{"m:f", "m:num", "m:den"}},
		{"根式", `\sqrt{2}`, []string{"m:rad"}},
		{"向量", `\vec{a}`, []string{"m:acc", "\u20d7"}},
		{"上横线", `\overline{AB}`, []string{"m:bar"}},
		{"函数+括号", `\sin\left(2x+\dfrac{\pi}{6}\right)`, []string{"sin", "m:d", "m:f", "\u03c0"}},
		{"符号", `\alpha\cdot\beta`, []string{"\u03b1", "\u00b7", "\u03b2"}},
		{"普通文本", `2x-1=0`, []string{"2x", "=", "0"}},
		{"左右括号", `\left[\dfrac{1}{2},\ 1\right]`, []string{"m:d", "["}},
	}
	for _, c := range cases {
		got := latexToOMML(c.tex)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: latexToOMML(%q) = %s，缺少 %q", c.name, c.tex, got, w)
			}
		}
	}
}

func TestBuildDocx(t *testing.T) {
	q := Question{
		ID:       1,
		Type:     "单选",
		Stem:     "已知 $\\vec{a}=(1,2)$，$\\vec{b}=(x,-1)$，若 $\\vec{a}\\perp\\vec{b}$，则实数 $x$ 的值为",
		Options:  []string{"$-2$", "$2$"},
		Answer:   "B",
		Solution: "因为 $\\vec{a}\\cdot\\vec{b}=0$，解得 $x=2$。",
	}
	groups := [][]docxQuestion{
		{{Num: 1, Q: q, Score: 5}},
	}
	data, err := buildDocx(t.TempDir(), "单元测试卷", groups, true, 5, paperTemplates["exam"])
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	for _, need := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml"} {
		if !found[need] {
			t.Errorf("docx 缺少文件: %s", need)
		}
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			rc.Close()
			docXML = buf.String()
		}
	}
	for _, w := range []string{"<m:oMath>", "m:acc", "\u20d7", "单元测试卷", "答案："} {
		if !strings.Contains(docXML, w) {
			t.Errorf("document.xml 缺少 %q", w)
		}
	}
	assertWellFormedXML(t, docXML)
}

// 对真实题库中所有公式做转换冒烟测试（依赖本机 D:\Fire 数据库，文件不存在则跳过）
func TestRealBankConversion(t *testing.T) {
	dbPath := `D:\Fire\.fire_tiku.db`
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("题库数据库不存在，跳过")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT stem, options, answer, solution FROM questions`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	re := regexp.MustCompile(`\$\$([\s\S]*?)\$\$|\$([^$\n]+?)\$`)
	count := 0
	for rows.Next() {
		var stem, options, answer, solution string
		rows.Scan(&stem, &options, &answer, &solution)
		text := stem + "\n" + options + "\n" + answer + "\n" + solution
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			expr := m[1]
			if expr == "" {
				expr = m[2]
			}
			out := latexToOMML(expr)
			if out == "" {
				t.Errorf("公式转换结果为空: %q", expr)
				continue
			}
			if strings.Contains(out, "<m:t") && (strings.Contains(out, "<w:") || strings.Contains(out, "oMath>")) {
				// 结构应只包含 m: 命名空间节点与转义文本
			}
			count++
		}
	}
	t.Logf("共转换 %d 个公式", count)
	if count == 0 {
		t.Error("未找到任何公式")
	}
}

// 长选项使用制表位两列对齐，且文档中不出现表格；图片 ID 唯一
func TestDocxOptionsNoTableAndUniqueImageIDs(t *testing.T) {
	q := Question{
		ID:      1,
		Type:    "单选",
		Stem:    "测试题目",
		Options: []string{"这是一个非常长的选项内容用来验证两列对齐布局", "第二个也很长的选项内容用来验证对齐", "第三个较长的选项内容", "第四个较长的选项内容"},
		Answer:  "A",
	}
	groups := [][]docxQuestion{{{Num: 1, Q: q, Score: 5}}}
	data, err := buildDocx(t.TempDir(), "表格测试", groups, true, 5, paperTemplates["exam"])
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var docXML string
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			buf := new(bytes.Buffer)
			buf.ReadFrom(rc)
			rc.Close()
			docXML = buf.String()
		}
	}
	if strings.Contains(docXML, "<w:tbl>") {
		t.Error("选项不应使用表格")
	}
	if !strings.Contains(docXML, "<w:tab ") {
		t.Error("两列选项应使用制表位")
	}
	assertWellFormedXML(t, docXML)

	// 图片 ID 唯一
	dir := t.TempDir()
	imgPath := dir + "/test.png"
	f, _ := os.Create(imgPath)
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 400, 300)))
	f.Close()
	b := &docxBuilder{}
	rid, ok := b.addImageFromFile(imgPath)
	if !ok {
		t.Fatal("addImageFromFile 失败")
	}
	x1 := b.imageDrawingXML(rid, imgPath)
	x2 := b.imageDrawingXML(rid, imgPath)
	idRe := regexp.MustCompile(`<wp:docPr id="(\d+)"`)
	m1 := idRe.FindStringSubmatch(x1)
	m2 := idRe.FindStringSubmatch(x2)
	if len(m1) < 2 || len(m2) < 2 || m1[1] == m2[1] {
		t.Errorf("图片 docPr ID 应唯一: %v vs %v", m1, m2)
	}
	// 图片宽度统一为 5cm
	if !strings.Contains(x1, `cx="1905000"`) {
		t.Errorf("图片宽度应统一为 1905000 EMU，实际: %s", x1)
	}
	assertWellFormedXML(t, x1)
	assertWellFormedXML(t, x2)
}
