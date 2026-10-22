---
title: 跟随
description: 让叙述转而跟随某个人物，从其视角与所知继续讲述。
args:
  - {name: who, hint: 人物名, options: [沈遥, 林舟, 老陈, 周看守], required: true}
present: [slash, button]
icon: eye
---
（玩家希望叙述此后跟随 {{ args.who }}。请把镜头自然地转到 {{ args.who }} 身上，从其视角与所知继续讲述，并把 follow 更新为 {{ args.who }}。）
{{ input }}
