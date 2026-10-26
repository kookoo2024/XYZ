package tiku

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"xyz/internal/safepath"
)

// ===================== LaTeX → OMML =====================

var latexSymbols = map[string]string{
	"alpha": "\u03b1", "beta": "\u03b2", "gamma": "\u03b3", "delta": "\u03b4",
	"epsilon": "\u03b5", "varepsilon": "\u03b5", "zeta": "\u03b6", "eta": "\u03b7",
	"theta": "\u03b8", "vartheta": "\u03d1", "iota": "\u03b9", "kappa": "\u03ba",
	"lambda": "\u03bb", "mu": "\u03bc", "nu": "\u03bd", "xi": "\u03be",
	"pi": "\u03c0", "rho": "\u03c1", "sigma": "\u03c3", "tau": "\u03c4",
	"upsilon": "\u03c5", "phi": "\u03c6", "varphi": "\u03c6", "chi": "\u03c7",
	"psi": "\u03c8", "omega": "\u03c9",
	"Gamma": "\u0393", "Delta": "\u0394", "Theta": "\u0398", "Lambda": "\u039b",
	"Xi": "\u039e", "Pi": "\u03a0", "Sigma": "\u03a3", "Upsilon": "\u03a5",
	"Phi": "\u03a6", "Psi": "\u03a8", "Omega": "\u03a9",
	"times": "\u00d7", "div": "\u00f7", "cdot": "\u00b7", "pm": "\u00b1",
	"mp": "\u2213", "leq": "\u2264", "geq": "\u2265", "neq": "\u2260",
	"approx": "\u2248", "equiv": "\u2261", "sim": "\u223c", "propto": "\u221d",
	"perp": "\u22a5", "parallel": "\u2225", "in": "\u2208", "notin": "\u2209",
	"subset": "\u2282", "supset": "\u2283", "subseteq": "\u2286", "supseteq": "\u2287",
	"cup": "\u222a", "cap": "\u2229", "emptyset": "\u2205", "varnothing": "\u2205",
	"infty": "\u221e", "angle": "\u2220", "triangle": "\u25b3", "therefore": "\u2234",
	"because": "\u2235", "circ": "\u2218", "degree": "\u00b0", "prime": "\u2032",
	"ldots": "\u2026", "cdots": "\u22ef", "dots": "\u2026", "partial": "\u2202",
	"nabla": "\u2207", "forall": "\u2200", "exists": "\u2203", "neg": "\u00ac",
	"wedge": "\u2227", "vee": "\u2228", "oplus": "\u2295", "otimes": "\u2297",
	"ast": "\u2217", "star": "\u22c6", "leftrightarrow": "\u2194",
	"rightarrow": "\u2192", "leftarrow": "\u2190", "Rightarrow": "\u21d2",
	"Leftarrow": "\u21d0", "Leftrightarrow": "\u21d4", "uparrow": "\u2191",
	"downarrow": "\u2193", "angleABC": "\u2220",
	"lbrace": "{", "rbrace": "}", "langle": "\u27e8", "rangle": "\u27e9",
	"lfloor": "\u230a", "rfloor": "\u230b", "lceil": "\u2308", "rceil": "\u2309",
	"backslash": "\\", "vert": "|", "Vert": "\u2016", "aleph": "\u2135",
	"hbar": "\u210f", "ell": "\u2113", "Re": "\u211c", "Im": "\u2111",
	"mathbb": "", "mathbf": "", "mathrm": "", "mathit": "", "boldsymbol": "",
}

var funcNames = map[string]bool{
	"sin": true, "cos": true, "tan": true, "cot": true, "sec": true, "csc": true,
	"arcsin": true, "arccos": true, "arctan": true, "log": true, "ln": true,
	"lg": true, "exp": true, "max": true, "min": true, "lim": true, "gcd": true,
	"mod": true, "det": true, "deg": true, "arg": true, "dim": true, "ker": true,
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func xmlEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func omRun(s string) string {
	return "<m:r><m:t xml:space=\"preserve\">" + xmlEscape(s) + "</m:t></m:r>"
}

func omPlain(s string) string {
	return "<m:r><m:rPr><m:sty m:val=\"p\"/></m:rPr><m:t xml:space=\"preserve\">" + xmlEscape(s) + "</m:t></m:r>"
}

func omFrac(num, den string) string {
	return "<m:f><m:fPr><m:type m:val=\"bar\"/></m:fPr><m:num>" + num + "</m:num><m:den>" + den + "</m:den></m:f>"
}

func omRad(deg, body string) string {
	if deg == "" {
		return "<m:rad><m:radPr><m:degHide m:val=\"1\"/></m:radPr><m:deg/><m:e>" + body + "</m:e></m:rad>"
	}
	return "<m:rad><m:radPr><m:degHide m:val=\"0\"/></m:radPr><m:deg>" + deg + "</m:deg><m:e>" + body + "</m:e></m:rad>"
}

func omAcc(chr, body string) string {
	return "<m:acc><m:accPr><m:chr m:val=\"" + xmlEscape(chr) + "\"/></m:accPr><m:e>" + body + "</m:e></m:acc>"
}

func omBar(body string) string {
	return "<m:bar><m:barPr><m:pos m:val=\"top\"/></m:barPr><m:e>" + body + "</m:e></m:bar>"
}

func omDelim(beg, end, inner string) string {
	if beg == "" && end == "" {
		return inner
	}
	dPr := ""
	if beg != "" {
		dPr += "<m:begChr m:val=\"" + xmlEscape(beg) + "\"/>"
	}
	if end != "" {
		dPr += "<m:endChr m:val=\"" + xmlEscape(end) + "\"/>"
	}
	return "<m:d><m:dPr>" + dPr + "</m:dPr><m:e>" + inner + "</m:e></m:d>"
}

func wrapSubSup(base, sub, sup string) string {
	if sub != "" && sup != "" {
		return "<m:sSubSup><m:e>" + base + "</m:e><m:sub>" + sub + "</m:sub><m:sup>" + sup + "</m:sup></m:sSubSup>"
	}
	if sup != "" {
		return "<m:sSup><m:e>" + base + "</m:e><m:sup>" + sup + "</m:sup></m:sSup>"
	}
	if sub != "" {
		return "<m:sSub><m:e>" + base + "</m:e><m:sub>" + sub + "</m:sub></m:sSub>"
	}
	return base
}

type latexParser struct {
	s       string
	i       int
	stopped bool
}

func (p *latexParser) eof() bool { return p.i >= len(p.s) }

func (p *latexParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.s[p.i]
}

func (p *latexParser) next() byte {
	if p.eof() {
		return 0
	}
	c := p.s[p.i]
	p.i++
	return c
}

func isOpChar(c byte) bool {
	switch c {
	case '+', '-', '=', '<', '>', '(', ')', '[', ']', ',', ';', '.', '!', '?', ':', '/', '|', '&':
		return true
	}
	return false
}

func (p *latexParser) parseExpr() []string {
	var out []string
	for !p.eof() && !p.stopped {
		c := p.peek()
		if c == '}' || c == '$' {
			break
		}
		atom := p.parseAtom()
		if atom == "" {
			break
		}
		// 上下标
		for !p.eof() && !p.stopped && (p.peek() == '^' || p.peek() == '_') {
			op := p.next()
			var sub, sup string
			if op == '_' {
				sub = p.parseAtom()
			} else {
				sup = p.parseAtom()
			}
			if !p.eof() && !p.stopped && (p.peek() == '^' || p.peek() == '_') {
				op2 := p.next()
				if op2 == '_' {
					sub = p.parseAtom()
				} else {
					sup = p.parseAtom()
				}
			}
			atom = wrapSubSup(atom, sub, sup)
		}
		out = append(out, atom)
	}
	return out
}

func (p *latexParser) parseAtom() string {
	if p.eof() || p.stopped {
		return ""
	}
	c := p.peek()
	switch {
	case c == '{':
		p.next()
		inner := strings.Join(p.parseExpr(), "")
		if p.peek() == '}' {
			p.next()
		}
		return inner
	case c == '\\':
		return p.parseCommand()
	case c == '^' || c == '_':
		p.next()
		return ""
	case isOpChar(c):
		p.next()
		return omRun(string(c))
	default:
		var b strings.Builder
		for !p.eof() && !p.stopped {
			c = p.peek()
			if c == '\\' || c == '{' || c == '}' || c == '^' || c == '_' || c == '$' || isOpChar(c) {
				break
			}
			b.WriteByte(c)
			p.next()
		}
		return omRun(b.String())
	}
}

func (p *latexParser) parseCommand() string {
	if p.eof() {
		return ""
	}
	p.next() // 吃掉反斜杠
	var name string
	for !p.eof() && isAlpha(p.peek()) {
		name += string(p.next())
	}
	if name == "" {
		c := p.next()
		switch c {
		case '{':
			return omRun("{")
		case '}':
			return omRun("}")
		case '$':
			return omRun("$")
		case '%':
			return omRun("%")
		case '&':
			return omRun("&")
		case '#':
			return omRun("#")
		case '_':
			return omRun("_")
		case '\\':
			return omRun("\\\\")
		case ' ':
			return omRun(" ")
		case ',', ';', ':':
			return omRun(" ")
		case '!':
			return ""
		default:
			return omRun("\\" + string(c))
		}
	}
	if name == "right" {
		p.parseDelim()
		p.stopped = true
		return ""
	}
	if sym, ok := latexSymbols[name]; ok {
		if name == "mathbb" || name == "mathbf" || name == "mathrm" || name == "mathit" || name == "boldsymbol" {
			// 字体命令：直接取参数内容（Word 数学会自动处理样式）
			return p.parseAtom()
		}
		return omRun(sym)
	}
	switch name {
	case "frac", "dfrac", "tfrac", "cfrac":
		num := p.parseAtom()
		den := p.parseAtom()
		return omFrac(num, den)
	case "sqrt":
		deg := ""
		if p.peek() == '[' {
			p.next()
			deg = strings.Join(p.parseExpr(), "")
			if p.peek() == ']' {
				p.next()
			}
		}
		return omRad(deg, p.parseAtom())
	case "vec", "overrightarrow":
		return omAcc("\u20d7", p.parseAtom())
	case "bar", "overline":
		return omBar(p.parseAtom())
	case "underline":
		return omBar(p.parseAtom())
	case "hat":
		return omAcc("^", p.parseAtom())
	case "text":
		return omPlain(p.parseAtom())
	case "left":
		beg, end := p.parseDelim()
		inner := p.parseDelimited()
		return omDelim(beg, end, inner)
	case "begin", "end":
		if p.peek() == '{' {
			p.next()
			for !p.eof() && p.peek() != '}' {
				p.next()
			}
			if p.peek() == '}' {
				p.next()
			}
		}
		if name == "end" {
			p.stopped = true
		}
		return ""
	case "limits", "nolimits", "displaystyle", "textstyle", "scriptstyle", "scriptsize", "large", "big", "Big", "bigg", "Bigg", "bigl", "bigr", "Bigl", "Bigr", "biggl", "biggr", "left.", "right.", "quad", "qquad", "enspace", "thinspace":
		return ""
	case ",":
		return omRun(" ")
	case ";", ":":
		return omRun(" ")
	case " ":
		return omRun(" ")
	}
	if funcNames[name] {
		return omPlain(name)
	}
	// 未知命令：保留原文，保证可编辑
	return omRun("\\" + name)
}

func (p *latexParser) parseDelim() (string, string) {
	if p.eof() {
		return "", ""
	}
	c := p.peek()
	if c == '\\' {
		p.next()
		if p.eof() {
			return "", ""
		}
		d := p.next()
		switch d {
		case '{':
			return "{", "}"
		case '}':
			return "}", "}"
		case '|':
			return "|", "|"
		case '.':
			return "", ""
		case '(':
			return "(", ")"
		case ')':
			return ")", ")"
		case '[':
			return "[", "]"
		case ']':
			return "]", "]"
		case 'l', 'r':
			// \lbrace \rbrace \langle \rangle 等（已消费首字母）
			var nm string
			nm = string(d)
			for !p.eof() && isAlpha(p.peek()) {
				nm += string(p.next())
			}
			switch nm {
			case "lbrace", "rbrace":
				return "{", "}"
			case "langle":
				return "\u27e8", "\u27e9"
			case "rangle":
				return "\u27e9", "\u27e9"
			case "lfloor":
				return "\u230a", "\u230b"
			case "rfloor":
				return "\u230b", "\u230b"
			case "lceil":
				return "\u2308", "\u2309"
			case "rceil":
				return "\u2309", "\u2309"
			}
			return "", ""
		}
		return "", ""
	}
	d := p.next()
	switch d {
	case '(':
		return "(", ")"
	case ')':
		return ")", ")"
	case '[':
		return "[", "]"
	case ']':
		return "]", "]"
	case '|':
		return "|", "|"
	case '.':
		return "", ""
	default:
		return string(d), string(d)
	}
}

func (p *latexParser) parseDelimited() string {
	p.stopped = false
	var parts []string
	for !p.eof() && !p.stopped {
		c := p.peek()
		if c == '}' {
			break
		}
		parts = append(parts, p.parseExpr()...)
	}
	return strings.Join(parts, "")
}

func latexToOMML(expr string) string {
	p := &latexParser{s: expr}
	return strings.Join(p.parseExpr(), "")
}

// ===================== docx 打包 =====================

type docxMedia struct {
	name  string
	data  []byte
	ctype string
	ext   string
}

type docxBuilder struct {
	root     string // 数据根目录，用于解析 /files/ 图片
	media    []docxMedia
	mediaIdx int
	drawID   int
}

func (b *docxBuilder) addImageFromFile(absPath string) (string, bool) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	var ctype string
	switch ext {
	case ".png":
		ctype = "image/png"
	case ".jpg", ".jpeg":
		ctype = "image/jpeg"
	case ".gif":
		ctype = "image/gif"
	case ".webp":
		return "", false
	default:
		return "", false
	}
	_, _, err = image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", false
	}
	b.mediaIdx++
	extName := strings.TrimPrefix(ext, ".")
	if extName == "jpeg" {
		extName = "jpg"
	}
	name := fmt.Sprintf("image%d.%s", b.mediaIdx, extName)
	b.media = append(b.media, docxMedia{name: name, data: data, ctype: ctype, ext: extName})
	return fmt.Sprintf("rIdImg%d", b.mediaIdx), true
}

func (b *docxBuilder) imageSrcToPath(src string) string {
	src = strings.TrimSpace(src)
	if strings.HasPrefix(src, "/files/") {
		src = strings.TrimPrefix(src, "/files/")
		if u, err := url.PathUnescape(src); err == nil {
			src = u
		}
		abs, ok := safepath.Join(b.root, src)
		if !ok {
			return ""
		}
		return abs
	}
	// 可能是 data URI 或 http，跳过
	if strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return ""
	}
	abs, ok := safepath.Join(b.root, src)
	if !ok {
		return ""
	}
	return abs
}

// mixedToXML 把包含 $...$ / $$...$$ / <img> 的文本转成 w:r + m:oMath + w:drawing 混合 XML
func (b *docxBuilder) mixedToXML(text string) string {
	var sb strings.Builder
	re := regexp.MustCompile(`\$\$([\s\S]*?)\$\$|\$([^$\n]+?)\$|(<img[^>]*>)`)
	idx := 0
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		full := m[0]
		pos := strings.Index(text[idx:], full)
		if pos < 0 {
			break
		}
		pos += idx
		if pos > idx {
			sb.WriteString(textRunWithBr(text[idx:pos], false, 0))
		}
		switch {
		case m[1] != "" || m[2] != "":
			expr := m[1]
			if expr == "" {
				expr = m[2]
			}
			om := latexToOMML(expr)
			sb.WriteString("<m:oMath>" + om + "</m:oMath>")
		case m[3] != "":
			src := imgSrc(m[3])
			abs := b.imageSrcToPath(src)
			if abs != "" {
				if rid, ok := b.addImageFromFile(abs); ok {
					sb.WriteString(b.imageDrawingXML(rid, abs))
				}
			}
		}
		idx = pos + len(full)
	}
	if idx < len(text) {
		sb.WriteString(textRunWithBr(text[idx:], false, 0))
	}
	return sb.String()
}

func imgSrc(imgTag string) string {
	re := regexp.MustCompile(`src=["']([^"']+)["']`)
	m := re.FindStringSubmatch(imgTag)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func (b *docxBuilder) imageDrawingXML(rid, absPath string) string {
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return ""
	}
	const maxEMU = 1905000 // 约 5cm
	// 像素 → EMU（1px = 9525 EMU），小图不放大，大图限制在 5cm 内
	w := int64(cfg.Width) * 9525
	h := int64(cfg.Height) * 9525
	// 统一显示宽度：全部缩放到 5cm 宽，高度按比例，超高再限 5cm
	h = h * maxEMU / w
	w = maxEMU
	if h > maxEMU {
		w = w * maxEMU / h
		h = maxEMU
	}
	cx, cy := w, h
	b.drawID++
	drawID := b.drawID
	return "<w:r><w:drawing><wp:inline distT=\"0\" distB=\"0\" distL=\"0\" distR=\"0\" xmlns:wp=\"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing\">" +
		"<wp:extent cx=\"" + fmt.Sprint(cx) + "\" cy=\"" + fmt.Sprint(cy) + "\"/>" +
		"<wp:docPr id=\"" + fmt.Sprint(drawID) + "\" name=\"img" + fmt.Sprint(drawID) + "\"/>" +
		"<a:graphic xmlns:a=\"http://schemas.openxmlformats.org/drawingml/2006/main\">" +
		"<a:graphicData uri=\"http://schemas.openxmlformats.org/drawingml/2006/picture\">" +
		"<pic:pic xmlns:pic=\"http://schemas.openxmlformats.org/drawingml/2006/picture\">" +
		"<pic:nvPicPr><pic:cNvPr id=\"" + fmt.Sprint(drawID) + "\" name=\"img" + fmt.Sprint(drawID) + "\"/><pic:cNvPicPr/></pic:nvPicPr>" +
		"<pic:blipFill><a:blip r:embed=\"" + rid + "\"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>" +
		"<pic:spPr><a:xfrm><a:off x=\"0\" y=\"0\"/><a:ext cx=\"" + fmt.Sprint(cx) + "\" cy=\"" + fmt.Sprint(cy) + "\"/></a:xfrm>" +
		"<a:prstGeom prst=\"rect\"><a:avLst/></a:prstGeom></pic:spPr>" +
		"</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>"
}

// ===================== 文档生成 =====================

type docxQuestion struct {
	Num   int
	Q     Question
	Score float64
}

// PaperTemplate 排版模板配置
type PaperTemplate struct {
	Key        string
	Name       string
	WithHeader bool
	InfoFields []string
	GapLines   int // 解答题后留白行数（学生版）
	PageFooter bool
	Cols       int // 1=单栏 2=双栏
}

var paperTemplates = map[string]PaperTemplate{
	"exam": {
		Key: "exam", Name: "标准试卷", WithHeader: true,
		InfoFields: []string{"学校", "班级", "姓名", "学号"},
		GapLines:   5, PageFooter: true, Cols: 1,
	},
	"practice": {
		Key: "practice", Name: "课堂练习", WithHeader: true,
		InfoFields: []string{"姓名", "日期"},
		GapLines:   4, PageFooter: false, Cols: 1,
	},
	"special": {
		Key: "special", Name: "专题练习", WithHeader: false,
		GapLines: 4, PageFooter: false, Cols: 1,
	},
	"doubleCol": {
		Key: "doubleCol", Name: "双栏练习", WithHeader: false,
		InfoFields: []string{"姓名", "日期"},
		GapLines:   3, PageFooter: false, Cols: 2,
	},
	"quiz": {
		Key: "quiz", Name: "随堂小测", WithHeader: false,
		InfoFields: []string{"姓名", "日期"},
		GapLines:   2, PageFooter: false, Cols: 1,
	},
}

var defaultScoreByType = map[string]float64{
	"单选":  5,
	"多选":  6,
	"填空":  5,
	"解答题": 12,
}

func paragraphXML(runs string, center bool, bold bool, size int, indent int) string {
	var pPr strings.Builder
	if center {
		pPr.WriteString("<w:jc w:val=\"center\"/>")
	}
	if indent > 0 {
		pPr.WriteString(fmt.Sprintf("<w:ind w:left=\"%d\"/>", indent))
	}
	pPr.WriteString("<w:spacing w:line=\"360\" w:lineRule=\"auto\"/>")
	var rPr strings.Builder
	rPr.WriteString("<w:rFonts w:ascii=\"Times New Roman\" w:eastAsia=\"宋体\" w:hAnsi=\"Times New Roman\"/>")
	if bold {
		rPr.WriteString("<w:b/>")
	}
	if size > 0 {
		rPr.WriteString(fmt.Sprintf("<w:sz w:val=\"%d\"/><w:szCs w:val=\"%d\"/>", size, size))
	}
	return "<w:p><w:pPr>" + pPr.String() + "<w:rPr>" + rPr.String() + "</w:rPr></w:pPr>" + runs + "</w:p>"
}

func textRunXML(s string, bold bool, size int) string {
	var rPr strings.Builder
	rPr.WriteString("<w:rFonts w:ascii=\"Times New Roman\" w:eastAsia=\"宋体\" w:hAnsi=\"Times New Roman\"/>")
	if bold {
		rPr.WriteString("<w:b/>")
	}
	if size > 0 {
		rPr.WriteString(fmt.Sprintf("<w:sz w:val=\"%d\"/><w:szCs w:val=\"%d\"/>", size, size))
	}
	return "<w:r><w:rPr>" + rPr.String() + "</w:rPr><w:t xml:space=\"preserve\">" + xmlEscape(s) + "</w:t></w:r>"
}

func fmtScoreDocx(n float64) string {
	if n == float64(int64(n)) {
		return fmt.Sprintf("%d", int64(n))
	}
	return fmt.Sprintf("%g", n)
}

// ===================== 排版工具 =====================

var subQRe = regexp.MustCompile(`([^$\w^{_\\])(?:\((1|2|3|4|5|6|7|8|9|10|11|12|13|14|15|16)\)|（(1|2|3|4|5|6|7|8|9|10|11|12|13|14|15|16)）)`)
var subQMathRe = regexp.MustCompile(`\$\((1|2|3|4|5|6|7|8|9|10|11|12|13|14|15|16)\)\$`)

// 大题小问换行：(1)(2) 等编号前插入换行（避开数学公式上下文）
func formatSubQuestions(s string) string {
	s = subQRe.ReplaceAllString(s, "$1\n($2$3)")
	s = subQMathRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(m, "$("), ")$")
		return "\n$(" + inner + ")$"
	})
	return s
}

// 提取题干中的 <img>，返回清理后的文本与图片 src 列表
func extractImgTags(s string) (string, []string) {
	var imgs []string
	out := imgTagRe.ReplaceAllStringFunc(s, func(m string) string {
		if src := imgSrc(m); src != "" {
			imgs = append(imgs, src)
		}
		return ""
	})
	return out, imgs
}

// 估算文本在 A4 中的宽度（中文=1，ASCII≈0.55，公式按源码长度折算）
func estTextWidth(s string) float64 {
	// 去掉公式标记，公式按源码长度 * 0.55 估算
	cleaned := mathRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.Trim(m, "$ ")
		return strings.Repeat("x", int(float64(len(inner))*0.55)+2)
	})
	var w float64
	for _, r := range cleaned {
		if r < 128 {
			w += 0.55
		} else {
			w += 1.0
		}
	}
	return w
}

func optionsRunsXML(b *docxBuilder, opts []string) string {
	var runs strings.Builder
	for i, o := range opts {
		if i > 0 {
			runs.WriteString(textRunXML("　　", false, 28))
		}
		runs.WriteString(textRunXML(string(rune('A'+i))+". ", true, 28))
		runs.WriteString(b.mixedToXML(o))
	}
	return runs.String()
}

// 选项段落：单行可行则一行，否则 2 列表格（A、B 一行，C、D 一行，上下对齐）
func (b *docxBuilder) optionsParagraphXML(opts []string) string {
	if len(opts) == 0 {
		return ""
	}
	var total float64
	for _, o := range opts {
		total += estTextWidth(o) + 4
	}
	if total <= 42 {
		return paragraphXML(optionsRunsXML(b, opts), false, false, 28, 420)
	}
	// 2 列布局：A、B 一行，C、D 一行，用制表位对齐（无表格、无边框）
	rows := (len(opts) + 1) / 2
	var out strings.Builder
	for r := 0; r < rows; r++ {
		leftIdx := r * 2
		var runs strings.Builder
		runs.WriteString(textRunXML(string(rune('A'+leftIdx))+". ", true, 28))
		runs.WriteString(b.mixedToXML(opts[leftIdx]))
		if leftIdx+1 < len(opts) {
			runs.WriteString(textRunXML("\t", false, 28))
			runs.WriteString(textRunXML(string(rune('A'+leftIdx+1))+". ", true, 28))
			runs.WriteString(b.mixedToXML(opts[leftIdx+1]))
		}
		out.WriteString(`<w:p><w:pPr><w:tabs><w:tab w:val="left" w:pos="4200"/></w:tabs>` +
			`<w:ind w:left="420"/><w:spacing w:line="320" w:lineRule="auto"/></w:pPr>` + runs.String() + `</w:p>`)
	}
	return out.String()
}

// 表头信息栏：学校/班级/姓名/学号 等，填空式（细下划线仅作填写位）
func infoLineXML(fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	var runs strings.Builder
	for i, f := range fields {
		if i > 0 {
			runs.WriteString(textRunXML("　　", false, 24))
		}
		runs.WriteString(textRunXML(f+"：", false, 24))
		runs.WriteString(`<w:r><w:rPr>` +
			`<w:rFonts w:ascii="Times New Roman" w:eastAsia="宋体" w:hAnsi="Times New Roman"/>` +
			`<w:u w:val="single"/></w:rPr><w:t xml:space="preserve">　　　　　　</w:t></w:r>`)
	}
	return paragraphXML(runs.String(), true, false, 24, 0)
}

// 文本 run（支持 \n 换行）
func textRunWithBr(s string, bold bool, size int) string {
	parts := strings.Split(s, "\n")
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteString("</w:t></w:r><w:r><w:br/></w:r><w:r><w:t xml:space=\"preserve\">")
		}
		b.WriteString(xmlEscape(p))
	}
	var rPr strings.Builder
	rPr.WriteString("<w:rFonts w:ascii=\"Times New Roman\" w:eastAsia=\"宋体\" w:hAnsi=\"Times New Roman\"/>")
	if bold {
		rPr.WriteString("<w:b/>")
	}
	if size > 0 {
		rPr.WriteString(fmt.Sprintf("<w:sz w:val=\"%d\"/><w:szCs w:val=\"%d\"/>", size, size))
	}
	return "<w:r><w:rPr>" + rPr.String() + "</w:rPr><w:t xml:space=\"preserve\">" + b.String() + "</w:t></w:r>"
}

var mathRe = regexp.MustCompile(`\$\$[\s\S]*?\$\$|\$[^$\n]+?\$`)

var imgTagRe = regexp.MustCompile(`<img[^>]*>`)
var mdBoldRe = regexp.MustCompile(`\*\*([^*]*)\*\*`)

func cleanMarkdown(s string) string {
	s = mdBoldRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// ===================== Markdown 表格支持 =====================

type docSeg struct {
	kind string // text / table
	text string
	rows [][]string
}

func isMDSepRow(s string) bool {
	t := strings.Trim(s, "| \t")
	if t == "" || !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		if r != '-' && r != ':' && r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

func parseMDRow(s string) []string {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

func splitMDTables(s string) []docSeg {
	var out []docSeg
	var textBuf strings.Builder
	flush := func() {
		if textBuf.Len() > 0 {
			out = append(out, docSeg{kind: "text", text: textBuf.String()})
			textBuf.Reset()
		}
	}
	lines := strings.Split(s, "\n")
	i := 0
	isTbl := func(l string) bool {
		t := strings.TrimSpace(l)
		return strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|")
	}
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if isTbl(line) && i+1 < len(lines) && isTbl(lines[i+1]) {
			sep := strings.TrimSpace(lines[i+1])
			flush()
			rows := [][]string{parseMDRow(line)}
			i++
			if isMDSepRow(sep) {
				i++
			}
			for i < len(lines) && isTbl(lines[i]) {
				rows = append(rows, parseMDRow(lines[i]))
				i++
			}
			out = append(out, docSeg{kind: "table", rows: rows})
			continue
		}
		if textBuf.Len() > 0 {
			textBuf.WriteString("\n")
		}
		textBuf.WriteString(lines[i])
		i++
	}
	flush()
	return out
}

// Markdown 表格 → Word 表格（题目内容表格，带边框）
func (b *docxBuilder) mdTableToWordXML(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return ""
	}
	const totalW = 9000
	colW := totalW / cols
	var grid strings.Builder
	for i := 0; i < cols; i++ {
		grid.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, colW))
	}
	var body strings.Builder
	for ri, r := range rows {
		body.WriteString("<w:tr>")
		for ci := 0; ci < cols; ci++ {
			cell := ""
			if ci < len(r) {
				cell = r[ci]
			}
			tcPr := `<w:tcW w:w="` + fmt.Sprint(colW) + `" w:type="dxa"/>`
			if ri == 0 {
				tcPr += `<w:shd w:val="clear" w:fill="F2F2F2"/>`
			}
			body.WriteString(`<w:tc><w:tcPr>` + tcPr + `</w:tcPr><w:p><w:pPr><w:jc w:val="center"/><w:spacing w:line="280" w:lineRule="auto"/></w:pPr>` + b.mixedToXML(cell) + `</w:p></w:tc>`)
		}
		body.WriteString("</w:tr>")
	}
	borders := `<w:top w:val="single" w:sz="4" w:color="000000"/><w:left w:val="single" w:sz="4" w:color="000000"/>` +
		`<w:bottom w:val="single" w:sz="4" w:color="000000"/><w:right w:val="single" w:sz="4" w:color="000000"/>` +
		`<w:insideH w:val="single" w:sz="4" w:color="000000"/><w:insideV w:val="single" w:sz="4" w:color="000000"/>`
	return `<w:tbl><w:tblPr><w:tblW w:w="` + fmt.Sprint(totalW) + `" w:type="dxa"/><w:tblBorders>` + borders + `</w:tblBorders></w:tblPr><w:tblGrid>` + grid.String() + `</w:tblGrid>` + body.String() + `</w:tbl>`
}

func buildDocx(root string, name string, groups [][]docxQuestion, showAnswer bool, total float64, tpl PaperTemplate) ([]byte, error) {
	b := &docxBuilder{root: root}
	var body strings.Builder

	// 标题
	body.WriteString(paragraphXML(textRunXML(name, true, 32), true, false, 32, 0))
	if len(tpl.InfoFields) > 0 {
		body.WriteString(infoLineXML(tpl.InfoFields))
		body.WriteString(paragraphXML("", false, false, 0, 0))
	}
	// 元信息
	today := timeNowStr()
	meta := "满分 " + fmtScoreDocx(total) + " 分"
	if tpl.Key == "exam" {
		meta += " · 考试时间 120 分钟"
	}
	meta += " · " + today
	if showAnswer {
		meta += " · 教师版（含答案）"
	} else {
		meta += " · 学生版"
	}
	body.WriteString(paragraphXML(textRunXML(meta, false, 21), true, false, 21, 0))

	// 双栏模板：标题与信息在单栏节，题目进入双栏节
	if tpl.Cols == 2 {
		body.WriteString(`<w:p><w:pPr><w:sectPr><w:pgSz w:w="11906" w:h="16838"/>` +
			`<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/>` +
			`<w:cols w:num="1" w:space="480"/></w:sectPr></w:pPr></w:p>`)
	}

	groupNames := []string{"一、选择题", "二、填空题", "三、解答题"}
	for gi, group := range groups {
		if len(group) == 0 {
			continue
		}
		var gt float64
		for _, dq := range group {
			gt += dq.Score
		}
		title := groupNames[gi] + "（共 " + fmt.Sprint(len(group)) + " 题，" + fmtScoreDocx(gt) + " 分）"
		body.WriteString(paragraphXML(textRunXML(title, true, 28), false, true, 28, 0))

		for _, dq := range group {
			q := dq.Q
			// 小题换行 + 提取题干内图片（图片统一放题目末尾）
			stemText, extraImgs := extractImgTags(formatSubQuestions(q.Stem))
			// 题号行
			body.WriteString(paragraphXML(
				textRunXML(fmt.Sprintf("%d. ", dq.Num), true, 28)+
					textRunXML("（"+q.Type+"，"+fmtScoreDocx(dq.Score)+"分）", false, 24),
				false, false, 28, 0))
			// 题干内容：文本段落与表格交替
			for _, seg := range splitMDTables(stemText) {
				if seg.kind == "table" {
					body.WriteString(b.mdTableToWordXML(seg.rows))
					body.WriteString(paragraphXML("", false, false, 0, 0))
				} else {
					body.WriteString(paragraphXML(b.mixedToXML(seg.text), false, false, 28, 0))
				}
			}

			// 选项
			if len(q.Options) > 0 {
				body.WriteString(b.optionsParagraphXML(q.Options))
			}

			// 题目图片（题干中的图 + images 字段，统一放题目末尾）
			images := q.Images
			images = append(images, extraImgs...)
			for _, img := range images {
				abs := b.imageSrcToPath(img)
				if abs != "" {
					if rid, ok := b.addImageFromFile(abs); ok {
						body.WriteString(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr>` + b.imageDrawingXML(rid, abs) + `</w:p>`)
					}
				}
			}

			if showAnswer {
				if q.Answer != "" {
					body.WriteString(paragraphXML(textRunXML("答案："+cleanMarkdown(q.Answer), true, 26), false, true, 26, 420))
				}
				if q.Solution != "" {
					body.WriteString(paragraphXML(textRunXML("解析：", true, 26)+b.mixedToXML(cleanMarkdown(q.Solution)), false, false, 26, 420))
				}
			}

			// 解答题留白（学生版），供学生作答
			if !showAnswer && q.Type == "解答题" && tpl.GapLines > 0 {
				for i := 0; i < tpl.GapLines; i++ {
					body.WriteString(`<w:p><w:pPr><w:spacing w:line="360" w:lineRule="auto"/></w:pPr></w:p>`)
				}
			}
		}
	}

	footerRef := ""
	if tpl.PageFooter {
		footerRef = `<w:footerReference w:type="default" r:id="rIdFooter1"/>`
	}
	colsXML := `<w:cols w:num="1" w:space="480"/>`
	if tpl.Cols == 2 {
		colsXML = `<w:cols w:num="2" w:space="480"/>`
	}

	documentXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<w:body>` + body.String() + `
<w:sectPr>` + footerRef + `<w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/>` + colsXML + `</w:sectPr>
</w:body></w:document>`

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Default Extension="png" ContentType="image/png"/>
<Default Extension="jpg" ContentType="image/jpeg"/>
<Default Extension="jpeg" ContentType="image/jpeg"/>
<Default Extension="gif" ContentType="image/gif"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`
	if tpl.PageFooter {
		contentTypes = strings.Replace(contentTypes,
			`<Override PartName="/word/styles.xml"`,
			`<Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
<Override PartName="/word/styles.xml"`, 1)
	}

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

	styles := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Times New Roman" w:eastAsia="宋体" w:hAnsi="Times New Roman"/><w:sz w:val="24"/></w:rPr></w:rPrDefault></w:docDefaults>
<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style>
</w:styles>`

	var docRels strings.Builder
	docRels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	docRels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	if tpl.PageFooter {
		docRels.WriteString(`<Relationship Id="rIdFooter1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`)
	}
	for i, m := range b.media {
		docRels.WriteString(fmt.Sprintf(`<Relationship Id="rIdImg%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`, i+1, m.name))
	}
	docRels.WriteString(`</Relationships>`)

	footerXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:p><w:pPr><w:jc w:val="center"/></w:pPr>
<w:r><w:t xml:space="preserve">第 </w:t></w:r>
<w:r><w:fldChar w:fldCharType="begin"/></w:r>
<w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r>
<w:r><w:fldChar w:fldCharType="end"/></w:r>
<w:r><w:t xml:space="preserve"> 页</w:t></w:r>
</w:p></w:ftr>`

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip := func(name, content string) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write([]byte(content))
		return err
	}
	if err := writeZip("[Content_Types].xml", contentTypes); err != nil {
		return nil, err
	}
	if err := writeZip("_rels/.rels", rels); err != nil {
		return nil, err
	}
	if err := writeZip("word/document.xml", documentXML); err != nil {
		return nil, err
	}
	if err := writeZip("word/styles.xml", styles); err != nil {
		return nil, err
	}
	if tpl.PageFooter {
		if err := writeZip("word/footer1.xml", footerXML); err != nil {
			return nil, err
		}
	}
	if err := writeZip("word/_rels/document.xml.rels", docRels.String()); err != nil {
		return nil, err
	}
	for _, m := range b.media {
		f, err := zw.Create("word/media/" + m.name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(m.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func timeNowStr() string {
	return time.Now().Format("2006-01-02")
}

// ===================== API =====================

func (s *Service) handleTikuExportDocx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name     string `json:"name"`
		Template string `json:"template"`
		Items    []struct {
			QID   int64   `json:"qid"`
			Score float64 `json:"score"`
		} `json:"items"`
		ShowAnswer bool `json:"show_answer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad json"})
		return
	}
	if len(req.Items) == 0 {
		writeJSON(w, map[string]interface{}{"error": "items required"})
		return
	}
	if req.Name == "" {
		req.Name = "组卷"
	}
	tpl, ok := paperTemplates[req.Template]
	if !ok {
		tpl = paperTemplates["exam"]
	}

	// 查询题目并保持传入顺序
	qmap := make(map[int64]Question)
	for _, it := range req.Items {
		if _, ok := qmap[it.QID]; ok {
			continue
		}
		var q Question
		var optionsJSON, tagsJSON, imagesJSON string
		err := s.db.QueryRow(`
			SELECT id, type, stem, options, answer, solution, chapter, section, 
			       difficulty, tags, images, source, use_count, last_used, created_at, updated_at
			FROM questions WHERE id = ?`, it.QID).Scan(
			&q.ID, &q.Type, &q.Stem, &optionsJSON, &q.Answer, &q.Solution,
			&q.Chapter, &q.Section, &q.Difficulty,
			&tagsJSON, &imagesJSON, &q.Source, &q.UseCount, &q.LastUsed, &q.CreatedAt, &q.UpdatedAt)
		if err != nil {
			continue
		}
		json.Unmarshal([]byte(optionsJSON), &q.Options)
		json.Unmarshal([]byte(tagsJSON), &q.Tags)
		json.Unmarshal([]byte(imagesJSON), &q.Images)
		qmap[it.QID] = q
	}

	// 分组：选择（单选+多选）/ 填空 / 解答，连续编号
	groups := make([][]docxQuestion, 3)
	num := 0
	var total float64
	for _, it := range req.Items {
		q, ok := qmap[it.QID]
		if !ok {
			continue
		}
		gi := 2
		if q.Type == "填空" {
			gi = 1
		} else if q.Type == "单选" || q.Type == "多选" {
			gi = 0
		}
		score := it.Score
		if score <= 0 {
			score = defaultScoreByType[q.Type]
		}
		num++
		total += score
		groups[gi] = append(groups[gi], docxQuestion{Num: num, Q: q, Score: score})
	}

	data, err := buildDocx(s.root, req.Name, groups, req.ShowAnswer, total, tpl)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	filename := url.PathEscape(req.Name + ".docx")
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+filename)
	w.Write(data)
}
