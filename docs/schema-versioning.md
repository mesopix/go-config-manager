# Schema 版本管理规范

## 核心原则

**已发布的 schema 文件永久保留，不得删除或修改。**

每个版本的 `schema.json` 一旦随程序发布并经过一段时间的生产验证，就必须作为历史快照永久保存在仓库中。这是后续配置文件版本升级的基础依据。

## 版本号说明

`schema.json` 中 `meta.version` 是**客户端的业务/schema 版本号**，与库的版本号（Git tag）无关。客户端自行定义和管理此版本号的语义和递增规则。

## schema.json 结构

```json
{
  "meta": {
    "version": "1.0.0"
  },
  "fields": {
    "name": {"Type": 0, "Required": true},
    "port": {"Type": 1, "Default": 8080}
  }
}
```

- `meta` — 元数据，至少包含 `version`，客户端可自行扩展其他字段
- `fields` — 字段定义集合，即 `Schema` 的序列化形式

## 历史快照目录结构

```
schemas/
├── 1.0.0.json     # 客户端首版 schema
├── 1.1.0.json     # 新增字段 / 类型变更
└── ...            # 每个有 schema 变更的版本一个文件
```

- 文件名格式：`<客户端schema版本号>.json`
- 仅在 schema 发生变化的版本创建新文件；无变化则复用上一版本
- 文件内容是该版本程序编译时嵌入的 `schema.json` 的完整副本

## 何时创建新版本文件

1. 新增配置字段
2. 删除或重命名配置字段
3. 修改字段类型（如 string → float）
4. 修改字段的 Required / Default 属性
5. 任何影响 `Check` / `Normalize` 行为的变更

## 用途

历史 schema 文件用于：

- **配置迁移**：对比相邻版本的 schema 差异，自动生成迁移逻辑
- **兼容性检查**：验证新版程序能否正确读取旧版配置文件
- **审计追溯**：追踪某个配置字段是在哪个版本引入或变更的
- **回滚参考**：需要降级时，找到对应版本的 schema 定义

## 注意事项

- 历史 schema 文件是**只读快照**，禁止事后修改
- 当前开发中的 schema 仍放在各模块目录下（如 `examples/demo/schema.json`），发布时才复制到 `schemas/`
- `schemas/` 目录下的文件应纳入版本控制，不要加入 `.gitignore`
