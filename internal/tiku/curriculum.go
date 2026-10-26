package tiku

import (
	"encoding/json"
	"net/http"

	"xyz/internal/store"
)

// CurriculumNode 教材目录树节点
type CurriculumNode struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"` // book / chapter / section / lesson
	Match    []string          `json:"match,omitempty"`
	Children []*CurriculumNode `json:"children,omitempty"`
}

// 默认人教 A 版（2019 课标）目录树，用户可在页面上增删改后保存
const defaultCurriculumJSON = `
[
  {"id":"b1","name":"必修第一册","type":"book","children":[
    {"id":"b1c1","name":"第一章 集合与常用逻辑用语","type":"chapter","match":["集合"],"children":[
      {"id":"b1c1s1","name":"1.1 集合的概念","type":"section"},
      {"id":"b1c1s2","name":"1.2 集合间的基本关系","type":"section"},
      {"id":"b1c1s3","name":"1.3 集合的基本运算","type":"section"},
      {"id":"b1c1s4","name":"1.4 充分条件与必要条件","type":"section"},
      {"id":"b1c1s5","name":"1.5 全称量词与存在量词","type":"section"}
    ]},
    {"id":"b1c2","name":"第二章 一元二次函数、方程和不等式","type":"chapter","match":["不等式"],"children":[
      {"id":"b1c2s1","name":"2.1 等式性质与不等式性质","type":"section"},
      {"id":"b1c2s2","name":"2.2 基本不等式","type":"section"},
      {"id":"b1c2s3","name":"2.3 二次函数与一元二次方程、不等式","type":"section"}
    ]},
    {"id":"b1c3","name":"第三章 函数的概念与性质","type":"chapter","match":["函数"],"children":[
      {"id":"b1c3s1","name":"3.1 函数的概念及其表示","type":"section"},
      {"id":"b1c3s2","name":"3.2 函数的基本性质","type":"section"},
      {"id":"b1c3s3","name":"3.3 幂函数","type":"section"},
      {"id":"b1c3s4","name":"3.4 函数的应用（一）","type":"section"}
    ]},
    {"id":"b1c4","name":"第四章 指数函数与对数函数","type":"chapter","match":["指数","对数"],"children":[
      {"id":"b1c4s1","name":"4.1 指数","type":"section"},
      {"id":"b1c4s2","name":"4.2 指数函数","type":"section"},
      {"id":"b1c4s3","name":"4.3 对数","type":"section"},
      {"id":"b1c4s4","name":"4.4 对数函数","type":"section"},
      {"id":"b1c4s5","name":"4.5 函数的应用（二）","type":"section"}
    ]},
    {"id":"b1c5","name":"第五章 三角函数","type":"chapter","match":["三角函数"],"children":[
      {"id":"b1c5s1","name":"5.1 任意角和弧度制","type":"section"},
      {"id":"b1c5s2","name":"5.2 三角函数的概念","type":"section"},
      {"id":"b1c5s3","name":"5.3 诱导公式","type":"section"},
      {"id":"b1c5s4","name":"5.4 三角函数的图象与性质","type":"section","match":["三角函数性质"]},
      {"id":"b1c5s5","name":"5.5 三角恒等变换","type":"section","match":["三角变换"]},
      {"id":"b1c5s6","name":"5.6 函数 y=Asin(ωx+φ)","type":"section"},
      {"id":"b1c5s7","name":"5.7 三角函数的应用","type":"section"}
    ]}
  ]},
  {"id":"b2","name":"必修第二册","type":"book","children":[
    {"id":"b2c6","name":"第六章 平面向量及其应用","type":"chapter","match":["向量"],"children":[
      {"id":"b2c6s1","name":"6.1 平面向量的概念","type":"section"},
      {"id":"b2c6s2","name":"6.2 平面向量的运算","type":"section","match":["向量运算","数量积","向量的数量积"],"children":[
        {"id":"b2c6s2l1","name":"6.2.1 向量的加法运算","type":"lesson"},
        {"id":"b2c6s2l2","name":"6.2.2 向量的减法运算","type":"lesson"},
        {"id":"b2c6s2l3","name":"6.2.3 向量的数乘运算","type":"lesson"},
        {"id":"b2c6s2l4","name":"6.2.4 向量的数量积","type":"lesson"}
      ]},
      {"id":"b2c6s3","name":"6.3 平面向量基本定理及坐标表示","type":"section","match":["数量积坐标","向量夹角","投影向量","坐标表示"],"children":[
        {"id":"b2c6s3l1","name":"6.3.1 平面向量基本定理","type":"lesson"},
        {"id":"b2c6s3l2","name":"6.3.2 平面向量的正交分解及坐标表示","type":"lesson"},
        {"id":"b2c6s3l3","name":"6.3.3 平面向量加、减运算的坐标表示","type":"lesson"},
        {"id":"b2c6s3l4","name":"6.3.4 平面向量数乘运算的坐标表示","type":"lesson"},
        {"id":"b2c6s3l5","name":"6.3.5 平面向量数量积的坐标表示","type":"lesson"}
      ]},
      {"id":"b2c6s4","name":"6.4 平面向量的应用","type":"section","match":["余弦定理","正弦定理","解三角形","向量应用","中线"],"children":[
        {"id":"b2c6s4l1","name":"6.4.1 平面几何中的向量方法","type":"lesson"},
        {"id":"b2c6s4l2","name":"6.4.2 向量在物理中的应用举例","type":"lesson"},
        {"id":"b2c6s4l3","name":"6.4.3 余弦定理、正弦定理","type":"lesson","match":["余弦定理","正弦定理"]},
        {"id":"b2c6s4l4","name":"6.4.3 余弦定理、正弦定理（第2课时）","type":"lesson"}
      ]}
    ]},
    {"id":"b2c7","name":"第七章 复数","type":"chapter","match":["复数"],"children":[
      {"id":"b2c7s1","name":"7.1 复数的概念","type":"section"},
      {"id":"b2c7s2","name":"7.2 复数的四则运算","type":"section","match":["复数四则运算","复数模","共轭复数","幂运算"]},
      {"id":"b2c7s3","name":"7.3 *复数的三角表示","type":"section"}
    ]},
    {"id":"b2c8","name":"第八章 立体几何初步","type":"chapter","match":["立体几何"],"children":[
      {"id":"b2c8s1","name":"8.1 基本立体图形","type":"section"},
      {"id":"b2c8s2","name":"8.2 立体图形的直观图","type":"section"},
      {"id":"b2c8s3","name":"8.3 简单几何体的表面积与体积","type":"section","match":["几何体表面积与体积"]},
      {"id":"b2c8s4","name":"8.4 空间点、直线、平面之间的位置关系","type":"section"},
      {"id":"b2c8s5","name":"8.5 空间直线、平面的平行","type":"section","match":["线面角","异面直线所成角","空间向量","正方体"]},
      {"id":"b2c8s6","name":"8.6 空间直线、平面的垂直","type":"section","match":["线面垂直","二面角"]}
    ]},
    {"id":"b2c9","name":"第九章 统计","type":"chapter","match":["统计"],"children":[
      {"id":"b2c9s1","name":"9.1 随机抽样","type":"section"},
      {"id":"b2c9s2","name":"9.2 用样本估计总体","type":"section"},
      {"id":"b2c9s3","name":"9.3 统计案例 公司员工的肥胖情况调查分析","type":"section"}
    ]},
    {"id":"b2c10","name":"第十章 概率","type":"chapter","match":["概率"],"children":[
      {"id":"b2c10s1","name":"10.1 随机事件与概率","type":"section"},
      {"id":"b2c10s2","name":"10.2 事件的相互独立性","type":"section"},
      {"id":"b2c10s3","name":"10.3 频率与概率","type":"section"}
    ]}
  ]},
  {"id":"b3","name":"选择性必修第一册","type":"book","children":[
    {"id":"b3c1","name":"第一章 空间向量与立体几何","type":"chapter","match":["空间向量与立体几何","空间向量"],"children":[
      {"id":"b3c1s1","name":"1.1 空间向量及其运算","type":"section"},
      {"id":"b3c1s2","name":"1.2 空间向量基本定理","type":"section"},
      {"id":"b3c1s3","name":"1.3 空间向量及其运算的坐标表示","type":"section"},
      {"id":"b3c1s4","name":"1.4 空间向量的应用","type":"section","match":["线面角","点到面距离","异面直线所成角"]}
    ]},
    {"id":"b3c2","name":"第二章 直线和圆的方程","type":"chapter","match":["直线","圆"],"children":[
      {"id":"b3c2s1","name":"2.1 直线的倾斜角与斜率","type":"section"},
      {"id":"b3c2s2","name":"2.2 直线的方程","type":"section"},
      {"id":"b3c2s3","name":"2.3 直线的交点坐标与距离公式","type":"section"},
      {"id":"b3c2s4","name":"2.4 圆的方程","type":"section"},
      {"id":"b3c2s5","name":"2.5 直线与圆、圆与圆的位置关系","type":"section"}
    ]},
    {"id":"b3c3","name":"第三章 圆锥曲线的方程","type":"chapter","match":["椭圆","双曲线","抛物线","圆锥曲线"],"children":[
      {"id":"b3c3s1","name":"3.1 椭圆","type":"section"},
      {"id":"b3c3s2","name":"3.2 双曲线","type":"section"},
      {"id":"b3c3s3","name":"3.3 抛物线","type":"section"}
    ]}
  ]},
  {"id":"b4","name":"选择性必修第二册","type":"book","children":[
    {"id":"b4c4","name":"第四章 数列","type":"chapter","match":["数列"],"children":[
      {"id":"b4c4s1","name":"4.1 数列的概念","type":"section"},
      {"id":"b4c4s2","name":"4.2 等差数列","type":"section"},
      {"id":"b4c4s3","name":"4.3 等比数列","type":"section"},
      {"id":"b4c4s4","name":"4.4 *数学归纳法","type":"section"}
    ]},
    {"id":"b4c5","name":"第五章 一元函数的导数及其应用","type":"chapter","match":["导数"],"children":[
      {"id":"b4c5s1","name":"5.1 导数的概念及其意义","type":"section"},
      {"id":"b4c5s2","name":"5.2 导数的运算","type":"section"},
      {"id":"b4c5s3","name":"5.3 导数在研究函数中的应用","type":"section"}
    ]}
  ]},
  {"id":"b5","name":"选择性必修第三册","type":"book","children":[
    {"id":"b5c6","name":"第六章 计数原理","type":"chapter","match":["计数原理","排列","组合"],"children":[
      {"id":"b5c6s1","name":"6.1 分类加法计数原理与分步乘法计数原理","type":"section"},
      {"id":"b5c6s2","name":"6.2 排列与组合","type":"section"},
      {"id":"b5c6s3","name":"6.3 二项式定理","type":"section"}
    ]},
    {"id":"b5c7","name":"第七章 随机变量及其分布","type":"chapter","match":["随机变量","分布"],"children":[
      {"id":"b5c7s1","name":"7.1 条件概率与全概率公式","type":"section"},
      {"id":"b5c7s2","name":"7.2 离散型随机变量及其分布列","type":"section"},
      {"id":"b5c7s3","name":"7.3 离散型随机变量的数字特征","type":"section"},
      {"id":"b5c7s4","name":"7.4 二项分布与超几何分布","type":"section"}
    ]},
    {"id":"b5c8","name":"第八章 成对数据的统计分析","type":"chapter","match":["回归","相关性","独立性检验"],"children":[
      {"id":"b5c8s1","name":"8.1 成对数据的相关关系","type":"section"},
      {"id":"b5c8s2","name":"8.2 一元线性回归模型及其应用","type":"section"},
      {"id":"b5c8s3","name":"8.3 列联表与独立性检验","type":"section"}
    ]}
  ]}
]
`

func defaultCurriculumTree() []*CurriculumNode {
	var tree []*CurriculumNode
	_ = json.Unmarshal([]byte(defaultCurriculumJSON), &tree)
	return tree
}

// GET /api/tiku/tree — 返回目录树（用户保存过则返回用户的，否则返回默认）
func (s *Service) handleTikuTreeGet(w http.ResponseWriter, r *http.Request) {
	data, ok := store.GetKV(s.db, store.KeyCurriculum)
	if !ok || data == "" {
		writeJSON(w, map[string]interface{}{"tree": defaultCurriculumTree()})
		return
	}
	var tree []*CurriculumNode
	if err := json.Unmarshal([]byte(data), &tree); err != nil || len(tree) == 0 {
		writeJSON(w, map[string]interface{}{"tree": defaultCurriculumTree()})
		return
	}
	writeJSON(w, map[string]interface{}{"tree": tree})
}

// POST /api/tiku/tree/save — 保存用户编辑后的目录树
func (s *Service) handleTikuTreeSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Tree []*CurriculumNode `json:"tree"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad json"})
		return
	}
	if len(req.Tree) == 0 {
		writeJSON(w, map[string]interface{}{"error": "tree required"})
		return
	}
	data, err := json.Marshal(req.Tree)
	if err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	if err := store.SetKV(s.db, store.KeyCurriculum, string(data)); err != nil {
		writeJSON(w, map[string]interface{}{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
