"""从 Obsidian 试卷 Markdown 导入题目到 XYZ 题库。

用法：
  python import_md.py            # dry-run：列出候选题
  python import_md.py --import   # 导入（默认选择带图题目，最多 20 道）
  python import_md.py --limit 20

题目结构遵循 math-tiku 规范：编号开头、**【答案】**、**【解析】**、<img> 标签。
"""

import glob
import json
import os
import re
import sys
import urllib.request

API = "http://localhost/api/tiku"
VAULT = r"D:\Fire"


def strip_md(s):
    """去除 Markdown 标题符号与多余空白"""
    s = re.sub(r"^#+\s*", "", s.strip())
    return re.sub(r"\s+", " ", s).strip()


def clean_md_bold(s):
    return re.sub(r"\*\*([^*]*)\*\*", r"\1", s or "").strip()


def fix_img_path(s, base_dir):
    """把选项/解析里的相对图片路径转成 /files/ 绝对 URL"""
    def repl(m):
        src = m.group(1)
        if src.startswith("/files/") or src.startswith("data:"):
            return m.group(0)
        abs_path = os.path.normpath(os.path.join(base_dir, src))
        rel = os.path.relpath(abs_path, VAULT)
        return '<img src="/files/' + rel.replace("\\", "/") + '" width="250">'
    return re.sub(r'<img[^>]*src="([^"]+)"[^>]*>', repl, s)


def convert_plain_table(text):
    """两行标签式纯文本表格（如：时刻\\n0:00\\n...\\n水深\\n10.0\\n...）转成 Markdown 表格"""
    lines = text.split("\n")
    out = []
    i = 0
    while i < len(lines):
        line = lines[i].strip()
        if 0 < len(line) <= 4 and not re.search(r"[0-9$|\\]", line):
            j = i + 1
            data1 = []
            while j < len(lines) and re.search(r"[0-9$]", lines[j]):
                data1.append(lines[j].strip())
                j += 1
            if data1 and j < len(lines):
                label2 = lines[j].strip()
                if 0 < len(label2) <= 4 and not re.search(r"[0-9$|\\]", label2):
                    k = j + 1
                    data2 = []
                    while k < len(lines) and re.search(r"[0-9$]", lines[k]):
                        data2.append(lines[k].strip())
                        k += 1
                    if len(data1) == len(data2) and len(data1) >= 2:
                        out.append("|" + line + "|" + "|".join(data1) + "|")
                        out.append("|" + "---|" * (len(data1) + 1))
                        out.append("|" + label2 + "|" + "|".join(data2) + "|")
                        i = k
                        continue
        out.append(lines[i])
        i += 1
    return "\n".join(out)


def parse_frontmatter(text):
    meta = {}
    m = re.match(r"^---\s*\n(.*?)\n---\s*\n", text, re.S)
    if m:
        for line in m.group(1).splitlines():
            if ":" in line:
                k, v = line.split(":", 1)
                meta[k.strip()] = v.strip().strip("'\"")
    return meta


def classify(stem, options, answer):
    """判断题型与章节"""
    if options:
        qtype = "多选" if len(answer) > 1 else "单选"
    elif "＿" in stem or "____" in stem or "___" in stem:
        qtype = "填空"
    else:
        qtype = "解答题"

    chapter = "未分类"
    section = ""
    s = stem
    if any(k in s for k in ["向量", r"\vec", "平面向量"]):
        chapter = "第六章 平面向量及其应用"
        if any(k in s for k in ["余弦", "正弦", "解三角形", "三角形面积"]):
            section = "6.4 平面向量的应用"
        elif "数量积" in s or "投影" in s:
            section = "6.2 平面向量的运算"
        elif "基本定理" in s or "坐标" in s:
            section = "6.3 平面向量基本定理及坐标表示"
        else:
            section = "6.1 平面向量的概念"
    elif any(k in s for k in ["复数", "虚数", "虚部", "共轭"]):
        chapter = "第七章 复数"
        section = "7.2 复数的四则运算" if "四则" in s or "运算" in s else "7.1 复数的概念"
    elif any(k in s for k in ["sin", "cos", "tan", "三角", "弧度", "周期", "三角函数", "摩天轮"]):
        chapter = "第五章 三角函数"
        if "恒等" in s or "变换" in s:
            section = "5.5 三角恒等变换"
        elif "图象" in s or "图像" in s or "周期" in s or "单调" in s:
            section = "5.4 三角函数的图象与性质"
        elif "诱导" in s:
            section = "5.3 诱导公式"
        else:
            section = "5.2 三角函数的概念"
    elif any(k in s for k in ["立体", "球体", "球面", "外接球", "内切球", "四面体", "正方体", "长方体",
                               "棱", "柱", "锥", "台", "截面", "直观图"]):
        chapter = "第八章 立体几何初步"
        if "表面" in s or "体积" in s or "外接球" in s or "内切球" in s:
            section = "8.3 简单几何体的表面积与体积"
        elif "平行" in s:
            section = "8.5 空间直线、平面的平行"
        elif "垂直" in s or "二面角" in s:
            section = "8.6 空间直线、平面的垂直"
        else:
            section = "8.1 基本立体图形"
    elif any(k in s for k in ["数列", "等差", "等比", "通项"]):
        chapter = "第四章 数列"
        if "等差" in s:
            section = "4.2 等差数列"
        elif "等比" in s:
            section = "4.3 等比数列"
        else:
            section = "4.1 数列的概念"
    elif any(k in s for k in ["概率", "统计", "样本", "抽样", "频率", "调查", "中位数", "方差", "众数", "平均数"]):
        chapter = "第十章 概率" if "概率" in s else "第九章 统计"
    elif any(k in s for k in ["集合", "充分", "必要", "命题"]):
        chapter = "第一章 集合与常用逻辑用语"
    elif any(k in s for k in ["函数", "单调", "奇偶", "零点", "指数", "对数", "幂"]):
        chapter = "第三章 函数的概念与性质"
        if "指数" in s:
            chapter = "第四章 指数函数与对数函数"
        if "对数" in s:
            chapter = "第四章 指数函数与对数函数"
    return qtype, chapter, section


def parse_paper(md_path):
    text = open(md_path, encoding="utf-8").read()
    meta = parse_frontmatter(text)
    source = meta.get("source") or meta.get("title") or os.path.splitext(os.path.basename(md_path))[0]
    fname = os.path.basename(md_path)
    base_dir = os.path.dirname(md_path)

    # 切分题目块：行首数字编号
    starts = [m.start() for m in re.finditer(r"(?m)^\s*(\d{1,2})[\.、]", text)]
    starts.append(len(text))
    questions = []
    for i in range(len(starts) - 1):
        block = text[starts[i]:starts[i + 1]]
        num = int(re.match(r"(?m)^\s*(\d{1,2})[\.、]", block).group(1))
        ans_m = re.search(r"【答案】", block)
        sol_m = re.search(r"【解析】", block)
        ans_start = ans_m.start() if ans_m else len(block)
        sol_start = sol_m.start() if sol_m else len(block)

        head = block[:ans_start]
        # 提取题干（编号后到选项区）
        body = re.sub(r"^\s*\d{1,2}[\.、]\s*", "", head).strip()
        opt_m = re.search(r"([A-D])\s*[\.．、]\s*", body)
        if opt_m and opt_m.start() < 300:
            stem = body[:opt_m.start()].strip()
            opt_text = body[opt_m.start():]
        else:
            stem = body
            opt_text = ""

        options = []
        if opt_text:
            opts = re.findall(r"([A-D])\s*[\.．、]\s*([^\nA-D]*(?:[A-D]?\s*[\.．、][^\nA-D]*)*)", opt_text)
            # 简单方式：按 A. B. C. D. 分割（支持同行连续选项）
            pieces = re.split(r"(?=[A-D]\s*[\.．、])", opt_text)
            for p in pieces:
                m = re.match(r"([A-D])\s*[\.．、]\s*(.*)", p.strip(), re.S)
                if m:
                    opt = fix_img_path(m.group(2), base_dir)
                    options.append(re.sub(r"\s+", " ", opt).strip())

        answer = ""
        if ans_m and sol_m and sol_start > ans_start:
            answer = clean_md_bold(re.sub(r"\s+", "", block[ans_start + len("【答案】"):sol_start]))
        elif ans_m:
            answer = clean_md_bold(re.sub(r"\s+", "", block[ans_start + len("【答案】"):]))

        solution = ""
        if sol_m:
            solution = clean_md_bold(re.sub(r"\s+", " ", block[sol_start + len("【解析】"):]))
            solution = fix_img_path(solution, base_dir)

        # 图片：仅提取题干中的 <img> 到题目末尾；解析中的图片保留在解析里
        images = []
        seen_img = set()
        for img in re.findall(r'<img[^>]*src="([^"]+)"[^>]*>', stem):
            if img.startswith("/files/"):
                if img not in seen_img:
                    seen_img.add(img)
                    images.append(img)
                continue
            abs_path = os.path.normpath(os.path.join(base_dir, img))
            rel = os.path.relpath(abs_path, VAULT)
            url = "/files/" + rel.replace("\\", "/")
            if url not in seen_img:
                seen_img.add(url)
                images.append(url)
        stem = re.sub(r"<img[^>]*>", "", stem).strip()
        stem = convert_plain_table(stem)

        qtype, chapter, section = classify(stem, options, answer)
        questions.append({
            "num": num,
            "type": qtype,
            "stem": stem,
            "options": options,
            "answer": answer,
            "solution": solution,
            "chapter": chapter,
            "section": section,
            "difficulty": "基础" if num <= 6 else ("中档" if num <= 13 else "拔高"),
            "tags": [],
            "images": images,
            "source": source,
            "file": fname,
        })
    return questions


def exists(stem):
    data = json_post("/list", {"page": 1, "page_size": 5, "keyword": stem[:20]})
    for q in data.get("questions", []):
        if q.get("stem") == stem:
            return True
    return False


def json_post(path, payload):
    req = urllib.request.Request(
        API + path,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read().decode("utf-8"))


def main():
    if "--fix-imported" in sys.argv:
        import sqlite3
        conn = sqlite3.connect(os.path.join(VAULT, ".xyz", "xyz.db"))
        c = conn.cursor()
        papers = sorted(glob.glob(os.path.join(VAULT, "【试卷】试卷收集", "*", "*.md")))
        updated = 0
        for p in papers:
            for q in parse_paper(p):
                row = c.execute("SELECT id FROM questions WHERE stem=? LIMIT 1", (q["stem"],)).fetchone()
                if not row:
                    continue
                c.execute("UPDATE questions SET images=?, solution=? WHERE id=?",
                          (json.dumps(q["images"], ensure_ascii=False), q["solution"], row[0]))
                updated += 1
        conn.commit()
        conn.close()
        print("--- 修复完成，更新", updated, "题 ---")
        return

    do_import = "--import" in sys.argv
    limit = 20
    if "--limit" in sys.argv:
        limit = int(sys.argv[sys.argv.index("--limit") + 1])

    papers = sorted(glob.glob(os.path.join(VAULT, "【试卷】试卷收集", "*", "*.md")))
    all_q = []
    for p in papers:
        try:
            all_q.extend(parse_paper(p))
        except Exception as e:
            print("解析失败:", p, e)

    # 优先带图、控制每卷数量、覆盖多章节
    from collections import Counter
    per_paper = Counter()
    chapter_used = Counter()
    picked = []
    for q in sorted(all_q, key=lambda x: -len(x["images"])):
        if len(picked) >= limit:
            break
        src = q["file"]
        if per_paper[src] >= 3:
            continue
        if chapter_used[q["chapter"]] >= 4:
            continue
        if q["chapter"] == "未分类":
            continue
        picked.append(q)
        per_paper[src] += 1
        chapter_used[q["chapter"]] += 1

    print("=== 候选题目 ===")
    for i, q in enumerate(picked):
        print(f"{i+1:2d}. [{q['type']}] {q['chapter']} | {q['file']} | 图{len(q['images'])}张 | {q['stem'][:38]}...")

    if not do_import:
        print("\n共", len(picked), "题。加 --import 导入。")
        return

    ok = 0
    for q in picked:
        try:
            r = json_post("/create", q)
            if r.get("id"):
                ok += 1
                print("OK #%s %s %s" % (r["id"], q["type"], q["stem"][:28]))
            else:
                print("FAIL:", r)
        except Exception as e:
            print("ERROR:", e)
    print("--- 完成，录入", ok, "题 ---")


if __name__ == "__main__":
    main()
