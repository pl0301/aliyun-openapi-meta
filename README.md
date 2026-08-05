# Aliyun OpenAPI Meta Data

阿里云 CLI 使用的统一 OpenAPI 元数据，目前主要用于
[aliyun-cli](https://github.com/aliyun/aliyun-cli) 构建。

## 数据布局

- `canonical/<product>/<version>/<api>.json`：统一的 API 元数据。
- `canonical/<product>/<version>/version.json`：对应产品版本的命令索引与中英文摘要。
- `metadatas/products.json`：产品索引、版本、Endpoint 和分发方式。
- `canonical-generation-status.json`：本次全量生成的范围、版本与失败记录。

原有 `metadatas` API 文件及 `en-US`、`zh-CN` 两套本地化 Meta 已合并进
`canonical`，不再单独发布。
