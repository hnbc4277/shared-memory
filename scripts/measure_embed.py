import json
import math
import urllib.request

payload = {
    "inputs": [
        "为这个句子生成表示以用于检索相关文章：用户喜欢用 Go 写后端 API",
        "用户偏好 Go 语言做服务端开发",
        "今天天气很好适合出门散步",
        "PostgreSQL 连接池配置",
        "The quick brown fox jumps over the lazy dog",
        "香蕉苹果西瓜草莓",
        "Go backend API preferred by user",
        "共享记忆系统需要按账号隔离并记录溯源",
    ],
    "truncate": True,
    "normalize": True,
}
req = urllib.request.Request(
    "http://127.0.0.1:18081/embed",
    data=json.dumps(payload).encode(),
    headers={"Content-Type": "application/json"},
)
vecs = json.load(urllib.request.urlopen(req))
labels = ["q", "related", "weather", "pg", "english", "fruit", "enrelated", "system"]
q = vecs[0]
print("dim", len(q), "norm", round(math.sqrt(sum(x * x for x in q)), 4))


def cos(a, b):
    return sum(x * y for x, y in zip(a, b))


for i, label in enumerate(labels):
    if i == 0:
        continue
    print(label, round(cos(q, vecs[i]), 4))
