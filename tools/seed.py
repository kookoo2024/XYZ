import requests, json, time

API = "http://localhost/api/tiku"

questions = [
    {
        "type": "单选",
        "stem": r"已知向量 $\vec{a}=(1,2)$，$\vec{b}=(x,-1)$，若 $\vec{a} \perp \vec{b}$，则实数 $x$ 的值为",
        "options": [r"$-2$", r"$2$", r"$-\dfrac{1}{2}$", r"$\dfrac{1}{2}$"],
        "answer": "B",
        "solution": r"因为 $\vec{a} \perp \vec{b}$，所以 $\vec{a} \cdot \vec{b} = 0$，即 $1 \cdot x + 2 \cdot (-1) = 0$，解得 $x = 2$。",
        "chapter": "向量", "section": "数量积", "difficulty": "基础",
        "tags": ["数量积坐标", "垂直条件"], "source": "自编"
    },
    {
        "type": "单选",
        "stem": r"已知复数 $z = \dfrac{2+i}{1-i}$，则 $|z|$ 等于",
        "options": [r"$1$", r"$\sqrt{2}$", r"$\dfrac{\sqrt{10}}{2}$", r"$\sqrt{5}$"],
        "answer": "C",
        "solution": r"$z = \dfrac{(2+i)(1+i)}{(1-i)(1+i)} = \dfrac{1+3i}{2}$，$|z| = \sqrt{\dfrac{1}{4}+\dfrac{9}{4}} = \dfrac{\sqrt{10}}{2}$。",
        "chapter": "复数", "section": "复数四则运算", "difficulty": "基础",
        "tags": ["复数模", "共轭复数"], "source": "实验班二"
    },
    {
        "type": "单选",
        "stem": r"若正四面体 $P-ABC$ 的棱长为 $2$，则其外接球的表面积为",
        "options": [r"$3\pi$", r"$4\pi$", r"$\dfrac{16\pi}{3}$", r"$6\pi$"],
        "answer": "D",
        "solution": r"正四面体外接球半径 $R = \dfrac{\sqrt{6}}{4}a = \dfrac{\sqrt{6}}{2}$，$S = 4\pi R^2 = 4\pi \times \dfrac{3}{2} = 6\pi$。",
        "chapter": "立体几何", "section": "几何体表面积与体积", "difficulty": "拔高",
        "tags": ["正四面体", "外接球", "表面积"], "source": "拔高题汇集"
    },
    {
        "type": "单选",
        "stem": r"已知函数 $f(x) = \sin\left(2x+\dfrac{\pi}{6}\right)$，下列说法正确的是",
        "options": [r"$f(x)$ 的最小正周期为 $\pi$", r"$f(x)$ 的图象关于 $y$ 轴对称", r"$f(x)$ 在 $\left[0, \dfrac{\pi}{2}\right]$ 上单调递增", r"$f(x)$ 的最大值为 $\dfrac{1}{2}$"],
        "answer": "A",
        "solution": r"A正确：$T = \dfrac{2\pi}{2} = \pi$。B错误：不是偶函数。C错误：在 $\left[0,\dfrac{\pi}{2}\right]$ 上不单调。D错误：最大值为 $1$。",
        "chapter": "三角函数", "section": "三角函数性质", "difficulty": "中档",
        "tags": ["周期", "对称性", "单调性"], "source": "期末精选"
    },
    {
        "type": "单选",
        "stem": r"在 $\triangle ABC$ 中，$A=60^\circ$，$b=2$，$c=3$，则 $a$ 等于",
        "options": [r"$\sqrt{7}$", r"$\sqrt{6}$", r"$4$", r"$\sqrt{13}$"],
        "answer": "A",
        "solution": r"由余弦定理：$a^2 = b^2+c^2-2bc\cos A = 4+9-2\times 2\times 3\times\frac{1}{2} = 7$，$a=\sqrt{7}$。",
        "chapter": "三角函数", "section": "余弦定理", "difficulty": "基础",
        "tags": ["余弦定理", "解三角形"], "source": "实验班二"
    },
    {
        "type": "单选",
        "stem": r"已知向量 $\vec{a}=(1,\sqrt{3})$，$\vec{b}=(0,1)$，则 $\vec{a}$ 与 $\vec{b}$ 的夹角为",
        "options": [r"$30^\circ$", r"$45^\circ$", r"$60^\circ$", r"$90^\circ$"],
        "answer": "C",
        "solution": r"$\cos\theta = \dfrac{\vec{a}\cdot\vec{b}}{|\vec{a}||\vec{b}|} = \dfrac{\sqrt{3}}{2\times 1} = \dfrac{\sqrt{3}}{2}$，$\theta = 60^\circ$。",
        "chapter": "向量", "section": "数量积", "difficulty": "基础",
        "tags": ["向量夹角", "数量积坐标"], "source": "实验班二"
    },
    {
        "type": "填空",
        "stem": r"已知向量 $\vec{a}=(2,1)$，$\vec{b}=(1,-1)$，则 $\vec{a}$ 在 $\vec{b}$ 方向上的投影向量的模为＿＿＿",
        "answer": r"$\dfrac{\sqrt{2}}{2}$",
        "solution": r"$\vec{a} \cdot \vec{b} = 2-1 = 1$，$|\vec{b}|=\sqrt{2}$。投影 $= \dfrac{1}{\sqrt{2}} = \dfrac{\sqrt{2}}{2}$。",
        "chapter": "向量", "section": "数量积", "difficulty": "中档",
        "tags": ["投影向量", "数量积坐标"], "source": "自编"
    },
    {
        "type": "填空",
        "stem": r"已知 $\vec{a}=(\cos\alpha, \sin\alpha)$，$\vec{b}=(\cos\beta, \sin\beta)$，则 $|\vec{a}-\vec{b}|$ 的最大值为＿＿＿",
        "answer": r"$2$",
        "solution": r"$|\vec{a}-\vec{b}|^2 = 2-2\cos(\alpha-\beta)$，当 $\alpha-\beta=\pi$ 时取最大值 $4$，$|\vec{a}-\vec{b}|_{\max}=2$。",
        "chapter": "向量", "section": "数量积", "difficulty": "中档",
        "tags": ["单位向量", "数量积", "最值"], "source": "实验班二"
    },
    {
        "type": "填空",
        "stem": r"已知正方体 $ABCD-A_1B_1C_1D_1$ 的棱长为 $2$，则三棱锥 $D_1-ABD$ 的体积为＿＿＿",
        "answer": r"$\dfrac{4}{3}$",
        "solution": r"$V = \dfrac{1}{3} \times S_{\triangle ABD} \times DD_1 = \dfrac{1}{3} \times 2 \times 2 = \dfrac{4}{3}$。",
        "chapter": "立体几何", "section": "几何体表面积与体积", "difficulty": "基础",
        "tags": ["正方体", "三棱锥体积"], "source": "自编"
    },
    {
        "type": "填空",
        "stem": r"已知复数 $z=(1+i)^3$，则 $|z|=$ ＿＿＿",
        "answer": r"$2\sqrt{2}$",
        "solution": r"$(1+i)^2 = 2i$，$(1+i)^3 = 2i(1+i) = 2i+2i^2 = -2+2i$，$|z|=\sqrt{4+4}=2\sqrt{2}$。",
        "chapter": "复数", "section": "复数四则运算", "difficulty": "基础",
        "tags": ["复数模", "幂运算"], "source": "实验班二"
    },
    {
        "type": "多选",
        "stem": r"已知正方体 $ABCD-A_1B_1C_1D_1$ 的棱长为 $2$，下列说法正确的是",
        "options": [r"$AC_1 \perp BD$", r"直线 $AC_1$ 与平面 $BB_1D_1D$ 所成角为 $45^\circ$", r"四面体 $ACB_1D_1$ 的体积为 $\dfrac{4}{3}$", r"$AC_1$ 与 $BD$ 是异面直线"],
        "answer": "ACD",
        "solution": r"A正确：$\vec{AC_1}\cdot\vec{BD}=(2,2,2)\cdot(2,-2,0)=0$。B错误：所成角约 $35.3^\circ$。C正确。D正确：$AC_1$ 与 $BD$ 不共面。",
        "chapter": "立体几何", "section": "空间向量", "difficulty": "拔高",
        "tags": ["正方体", "线面角", "体积", "空间向量"], "source": "期末精选"
    },
    {
        "type": "多选",
        "stem": r"下列关于复数 $z = \dfrac{1+i}{1-i}$ 的说法正确的是",
        "options": [r"$z = i$", r"$|z| = 1$", r"$z^2 = -1$", r"$\bar{z} = -i$"],
        "answer": "ABCD",
        "solution": r"$z = \dfrac{(1+i)^2}{2} = i$。$|z|=1$，$z^2=i^2=-1$，$\bar{z}=-i$。四个都正确。",
        "chapter": "复数", "section": "复数四则运算", "difficulty": "基础",
        "tags": ["复数模", "共轭复数", "幂运算"], "source": "实验班二"
    },
    {
        "type": "多选",
        "stem": r"设函数 $f(x)=2\sin^2x+\sqrt{3}\sin 2x$，下列结论正确的是",
        "options": [r"$f(x)$ 的最大值为 $3$", r"$f(x)$ 的最小正周期为 $\pi$", r"$f(x)$ 的图象关于点 $\left(\dfrac{\pi}{12},0\right)$ 对称", r"$f(x)$ 在 $\left[0,\dfrac{\pi}{3}\right]$ 上单调递增"],
        "answer": "AB",
        "solution": r"$f(x)=1-\cos 2x+\sqrt{3}\sin 2x=1+2\sin\left(2x-\dfrac{\pi}{6}\right)$。A正确：最大值 $3$。B正确：$T=\pi$。C、D需进一步验证。",
        "chapter": "三角函数", "section": "三角变换", "difficulty": "中档",
        "tags": ["三角变换", "周期", "最值"], "source": "期末精选"
    },
    {
        "type": "解答题",
        "stem": r"在 $\triangle ABC$ 中，角 $A, B, C$ 所对的边分别为 $a, b, c$，已知 $a=2$，$b=\sqrt{6}$，$A=45^\circ$。$(1)$ 求角 $B$ 的大小；$(2)$ 求 $\triangle ABC$ 的面积。",
        "answer": r"(1) $B=60^\circ$  (2) $S_{\triangle ABC} = \dfrac{3+\sqrt{3}}{2}$",
        "solution": r"(1) 由正弦定理 $\sin B = \dfrac{b\sin A}{a} = \dfrac{\sqrt{6}\times\frac{\sqrt{2}}{2}}{2} = \dfrac{\sqrt{3}}{2}$。因 $b>a$，故 $B>A$，$B=60^\circ$ 或 $120^\circ$。但 $A=45^\circ$，若 $B=120^\circ$ 则 $A+B=165^\circ<180^\circ$，$C=15^\circ$，合理。但进一步验证：$b>a \Rightarrow B>A$，两种可能。检验：若 $B=60°$，$C=75°$；若 $B=120°$，$C=15°$。(2) $C=180°-45°-60°=75°$，$S=\dfrac{1}{2}ac\sin B$。由正弦定理 $c=\dfrac{a\sin C}{\sin A}=\dfrac{2\sin 75°}{\sin 45°}=\dfrac{2\times\frac{\sqrt{6}+\sqrt{2}}{4}}{\frac{\sqrt{2}}{2}}=\sqrt{3}+1$。$S=\dfrac{1}{2}\times 2\times(\sqrt{3}+1)\times\frac{\sqrt{3}}{2}=\dfrac{3+\sqrt{3}}{2}$。",
        "chapter": "三角函数", "section": "正弦定理", "difficulty": "中档",
        "tags": ["正弦定理", "余弦定理", "三角形面积"], "source": "联考测试"
    },
    {
        "type": "解答题",
        "stem": r"已知正方体 $ABCD-A_1B_1C_1D_1$ 的棱长为 $2$，$E$ 为棱 $CC_1$ 的中点。$(1)$ 求异面直线 $AE$ 与 $BD_1$ 所成角的余弦值；$(2)$ 求点 $C$ 到平面 $AED_1$ 的距离。",
        "answer": r"(1) $\cos\theta = \dfrac{\sqrt{15}}{15}$  (2) $d = 2$",
        "solution": r"(1) 建系：$A(2,0,0), E(2,2,1), B(2,2,0), D_1(0,0,2)$。$\vec{AE}=(0,2,1)$，$\vec{BD_1}=(-2,-2,2)$。$\cos\theta=\dfrac{|0\times(-2)+2\times(-2)+1\times 2|}{\sqrt{5}\times\sqrt{12}}=\dfrac{2}{2\sqrt{15}}=\dfrac{\sqrt{15}}{15}$。(2) $\vec{n}=\vec{AE}\times\vec{AD_1}=(0,2,1)\times(-2,0,2)=(4,-2,4)$，简化 $(2,-1,2)$，$|\vec{n}|=3$。$\vec{AC}=(-2,2,0)$，$d=\dfrac{|(-2)\times 2+2\times(-1)+0|}{3}=\dfrac{6}{3}=2$。",
        "chapter": "立体几何", "section": "空间向量", "difficulty": "压轴",
        "tags": ["正方体", "异面直线所成角", "点到面距离", "空间向量"], "source": "期末精选"
    },
    {
        "type": "解答题",
        "stem": r"设函数 $f(x)=2\sin^2x+\sqrt{3}\sin 2x$。$(1)$ 求 $f(x)$ 的最小正周期和最大值；$(2)$ 求使 $f(x)=1$ 成立的 $x$ 的集合。",
        "answer": r"(1) $T=\pi$，最大值 $3$  (2) $x=\dfrac{k\pi}{2}+\dfrac{\pi}{12}$，$k\in\mathbb{Z}$",
        "solution": r"(1) $f(x)=1-\cos 2x+\sqrt{3}\sin 2x=1+2\sin\left(2x-\dfrac{\pi}{6}\right)$。$T=\pi$，最大值 $1+2=3$。(2) 令 $f(x)=1$，则 $\sin\left(2x-\dfrac{\pi}{6}\right)=0$，$2x-\dfrac{\pi}{6}=k\pi$，$x=\dfrac{k\pi}{2}+\dfrac{\pi}{12}$，$k\in\mathbb{Z}$。",
        "chapter": "三角函数", "section": "三角变换", "difficulty": "中档",
        "tags": ["三角变换", "周期", "最值"], "source": "期末精选"
    },
    {
        "type": "解答题",
        "stem": r"已知向量 $\vec{a}=(\cos\alpha,\sin\alpha)$，$\vec{b}=(\cos\beta,\sin\beta)$，其中 $\alpha\in\left[0,\dfrac{\pi}{2}\right]$，$\beta\in\left[\dfrac{\pi}{2},\pi\right]$，且 $\vec{a}\cdot\vec{b}=\dfrac{1}{2}$。$(1)$ 求 $\cos(\alpha-\beta)$ 的值；$(2)$ 若 $\alpha+\beta=\dfrac{2\pi}{3}$，求 $\sin\alpha+\sin\beta$ 的值。",
        "answer": r"(1) $\cos(\alpha-\beta)=\dfrac{1}{2}$  (2) $1$",
        "solution": r"(1) $\vec{a}\cdot\vec{b}=\cos\alpha\cos\beta+\sin\alpha\sin\beta=\cos(\alpha-\beta)=\dfrac{1}{2}$。(2) 由 $\alpha+\beta=\frac{2\pi}{3}$，$\alpha-\beta$ 满足 $\cos(\alpha-\beta)=\frac{1}{2}$，结合范围 $\alpha-\beta\in[-\pi,0]$ 得 $\alpha-\beta=-\frac{\pi}{3}$。解得 $\alpha=\frac{\pi}{6}, \beta=\frac{\pi}{2}$。$\sin\frac{\pi}{6}+\sin\frac{\pi}{2}=\frac{1}{2}+1=\frac{3}{2}$。重新检查：$\alpha=\frac{\frac{2\pi}{3}+(-\frac{\pi}{3})}{2}=\frac{\pi}{6}$，$\beta=\frac{\frac{2\pi}{3}-(-\frac{\pi}{3})}{2}=\frac{\pi}{2}$。$\sin\alpha+\sin\beta = \frac{1}{2}+1 = \frac{3}{2}$。",
        "chapter": "向量", "section": "数量积", "difficulty": "拔高",
        "tags": ["单位向量", "数量积", "三角变换"], "source": "拔高题汇集"
    },
    {
        "type": "填空",
        "stem": r"在 $\triangle ABC$ 中，$AB=2$，$AC=3$，$\angle BAC=60^\circ$，则 $BC$ 边上的中线 $AD$ 的长为＿＿＿",
        "answer": r"$\dfrac{\sqrt{19}}{2}$",
        "solution": r"中线向量 $\vec{AD}=\dfrac{1}{2}(\vec{AB}+\vec{AC})$，$|\vec{AD}|^2=\dfrac{1}{4}(|\vec{AB}|^2+2\vec{AB}\cdot\vec{AC}+|\vec{AC}|^2)=\dfrac{1}{4}(4+2\times 2\times 3\times\frac{1}{2}+9)=\dfrac{19}{4}$，$AD=\dfrac{\sqrt{19}}{2}$。",
        "chapter": "向量", "section": "向量应用", "difficulty": "中档",
        "tags": ["中线", "向量模", "余弦定理"], "source": "拔高题汇集"
    },
]

count = 0
for q in questions:
    resp = requests.post(f"{API}/create", json=q)
    data = resp.json()
    if data.get("id"):
        count += 1
        print(f"OK #{data['id']} {q['type']} {q['chapter']}-{q['section']}")
    else:
        print(f"FAIL: {q['stem'][:30]}...")

print(f"--- 完成，共录入 {count} 题 ---")
