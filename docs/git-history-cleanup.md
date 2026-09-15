# Git 历史清理前审计与回滚清单

> 本文是对当前本地克隆的只读审计记录和未来操作指南。此次审计**没有**改写提交历史、删除对象、删除远端内容，也没有 force push。

## 审计基线（2026-09-13；本地工作区路径不写入公开运维文档）

### refs、远程与可达性

- 当前命名 refs：`refs/heads/main` 与 `refs/remotes/origin/main` 均指向 `c8b615e689ef98958f57b39c2ef21be1a7f12cac`；没有 tag，也没有其他本地分支。
- `origin`：`https://github.com/GUYU2233/Alice-EVE.git`（fetch/push URL 相同）。远端可见 `HEAD` 和 `refs/heads/main` 都指向 `c8b615e...`；本次查询没有看到远端 tag 或其他分支。
- 当前 refs 可达对象中未发现旧 relay 二进制路径。
- 旧提交 `b8d1d2a...`、`c47eac3...`、`050b726...` 及其二进制 blob 已不被 `--no-reflogs` 的当前 refs 可达图引用，但仍可由本地 reflog 找到；`git fsck --full --no-reflogs --unreachable` 报告它们为 unreachable。只要 reflog 或其他引用仍存在，就不能视为已经从本地仓库清除。
- 远端 `ls-remote` 只能证明远端当前公开 refs，不足以证明 GitHub 后台缓存、PR refs、fork、已下载克隆或对象存储中不存在旧对象。

### 旧 relay 二进制对象

旧发布提交曾将下列路径写入 Git。它们当前均不在命名 refs 的工作树中：

| 历史路径 | Blob | 大小 |
| --- | --- | ---: |
| `relay-server/relay-linux` | `4616ba62c1dd1eced3cd87fc477423aba75daa25` | 15,451,769 bytes（约 14.73 MiB） |
| `relay-server/relay-prod` | `cc5dc8e77d2a2b7cd6c796898ccbc18d74ec118e` | 15,658,496 bytes（约 14.94 MiB） |
| `relay-server/relay-server/bin/relay-server` | `d307b347a0b7469b634f3451641c5df44f7200d2` | 10,594,304 bytes（约 10.11 MiB） |
| `relay-server/relay-server/relay-server.exe` | 与上一项相同 blob `d307b347...` | 10,594,304 bytes（约 10.11 MiB） |

其中两个 10,594,304-byte 路径复用了同一个 blob。旧提交总共引入约 41.7 MiB 的未压缩二进制内容（Git pack 实际占用会不同）。当前工作区仍可能有构建/发布文件，但它们受 ignore 规则控制；这与 Git 历史对象审计是两件事。

### releases/SHA256SUMS 引用核对

当前跟踪的 `releases/SHA256SUMS.txt` 有四项，均与本地 `releases/` 目录文件的 SHA-256 相符：

| 文件 | 工作区大小 | SHA-256 |
| --- | ---: | --- |
| `eve-assistant-mobile-android-v0.0.1-alpha.apk` | 50,364,502 bytes | `00761242EDDC7C9048576C8FFB33DCA61E523CF7848CE05BFDD082B89355382D` |
| `eve-assistant-mobile-windows.zip` | 12,041,720 bytes | `597817FDD4FB9F73000AFA4CDB71E8821131ECEBF6DD4D0DF7C26CAD4755E1F8` |
| `eve-assistant-v0.0.1-alpha.exe` | 11,998,208 bytes | `F2F193AD4BBC36C030A3580BDC35D07255C310F03002B1BB213DCAAA7169C687` |
| `eve-assistant-v0.0.1-alpha.zip` | 4,929,777 bytes | `2204A6EB7DCA07F484F4D83519DEC2B86501338A17EC9639C95FB9E3E0470151` |

这些二进制被 `.gitignore` 排除，清单只记录工作区/发布目录文件，不代表二进制已经提交或已经上传 GitHub。历史大写目录 `Releases/` 的清单曾包含 `relay-server.exe` 及旧路径/反斜杠引用；该目录已从当前命名 refs 移除，但其历史 blob 仍会保留在可恢复历史中，直到所有引用和保留副本处理完毕。

## 任何清理前的强制检查清单

1. **暂停发布和写入**：冻结合并、标签、Release 上传和自动镜像，记录审计基线（当前分支、所有 refs、远程 URL、HEAD SHA）。
2. **保护工作区**：在干净的临时克隆或独立备份副本中操作；先保存未提交变更和构建产物清单。不要在唯一工作副本运行清理。
3. **保存可回滚锚点**：导出 `git show-ref`、`git reflog --all`、`git count-objects -vH`、`git fsck --full` 输出，并在仓库外保存 `git bundle create alice-eve-before-cleanup.bundle --all --reflogs`（如环境支持）。校验 bundle 的 SHA-256。
4. **盘点全部引用**：检查本地分支、远端跟踪分支、tag、notes、stash、reflog，以及 `refs/pull/` 等显式/隐藏 refs（若从托管平台导出）。清理范围不能只写 `main`。
5. **确认目标集合**：用 `git rev-list --objects --all --reflog`、`git log --all --reflog -- <path>` 和 blob 大小报告确认待移除路径/对象；记录误删保护清单和必须保留的 release 文档。
6. **确认发布依赖**：盘点 GitHub Releases、Actions artifacts、包仓库、部署机、镜像、fork 和下游克隆。历史清理不会撤回已经下载的文件。
7. **选择工具与版本**：优先在副本使用固定版本的 `git-filter-repo`；BFG 需要 Java，且通常按 blob 内容/路径匹配，复杂路径或敏感数据场景必须先验证映射。不得直接在生产克隆试跑。
8. **先做模拟再验收**：记录工具命令、版本和日志；运行后检查所有 refs 的路径、blob 大小、提交数量、树结构、构建/测试与敏感内容扫描。确认 release 清单没有引用被移除的历史文件。
9. **准备协同窗口**：历史清理会改变所有受影响 commit ID；通知贡献者停止 push，准备重新克隆或按明确步骤重置本地分支。不要用普通 merge 把旧历史接回去。
10. **远端变更单独审批**：本地验收通过后，才讨论受保护分支的 ref replacement。除非仓库管理员明确批准，否则不执行 force push、删除远端分支/tag 或删除 Release。
11. **清理后验证保留对象**：在新克隆中验证目标 blob/path 不可达、当前源码和 release manifest 正确，运行 `git fsck --full --no-reflogs`；保留旧 bundle 和日志作为离线回滚材料。
12. **最后再处理本地垃圾回收**：确认回滚窗口、协作者和托管平台保留策略后，才考虑 reflog 过期和 `git gc`。过早 prune 会破坏本地回滚能力。

## 仅供批准后、临时副本使用的示例

### git-filter-repo（推荐先试跑）

```powershell
# 在临时 clone 中，先确认备份和基线
python -m pip install --user git-filter-repo

git filter-repo --analyze

git filter-repo --path relay-server/relay-linux `
  --path relay-server/relay-prod `
  --path relay-server/relay-server/bin/relay-server `
  --path relay-server/relay-server/relay-server.exe `
  --invert-paths

# 验收：不要直接覆盖原仓库
 git fsck --full --no-reflogs
 git log --all --reflog --name-only --pretty='' | Select-String 'relay-(linux|prod)|relay-server.exe|relay-server/bin/relay-server'
```

`git-filter-repo` 默认会重写提交并可能移除/重建 refs；命令必须在副本中按实际 refs 需求调整。若还要清理旧 `Releases/` 大写目录中的错误/过时条目，应先明确是否要保留其文档历史，再单独验证 path mapping。

### BFG Repo-Cleaner（备选）

```powershell
# 在临时 clone 中，使用固定版本 bfg.jar；不要对唯一副本执行
java -jar bfg.jar --delete-files relay-linux,relay-prod,relay-server.exe repo.git
# 或针对已确认的超大 blob：
# java -jar bfg.jar --strip-blobs-bigger-than 10M repo.git

git reflog expire --expire=now --all
git gc --prune=now --aggressive
 git fsck --full --no-reflogs
```

BFG 的按文件名清理可能匹配同名但应保留的文件；按大小清理可能误删合法大文件。`reflog expire`/`gc --prune=now` 是不可逆风险操作，只能在备份、验收和回滚窗口明确后执行。

## 风险与回滚

- **提交 ID 全面变化**：签名、构建证明、Issue/PR 引用、部署锁定 SHA、缓存键和外部 webhook 可能失效。
- **远端不是唯一副本**：force push 后旧对象可能仍存在于 fork、克隆、PR refs、托管平台缓存、Release 下载和备份；若内容含敏感信息，还必须轮换凭据/密钥并按事件响应流程处理。
- **reflog 与垃圾回收时序**：未过期 reflog 会使对象继续可达；过早 prune 会使本地回滚失效。不同托管平台的对象保留时间不可假设。
- **发布引用错配**：当前 `releases/SHA256SUMS.txt` 只对应四个被忽略的工作区产物；不要把旧 `Releases/` 清单中的 `relay-server.exe` 当作当前客户端 release。发布前重新生成并独立验证 checksum。
- **工具误删/规则过宽**：BFG 的大小阈值和文件名匹配都可能删除合法内容；filter-repo 的 path 规则也可能影响文档、测试夹具或部署脚本。必须对比 before/after manifest 并运行测试。

### 回滚原则（不改写远端）

在远端替换 refs 之前，最安全的回滚是删除临时副本、恢复原始工作副本/备份 bundle，并继续使用原始 refs。若已在本地副本重写但尚未推送，可从 bundle 恢复：

```powershell
git clone alice-eve-before-cleanup.bundle alice-eve-restore
# 或在明确要恢复的副本中：
 git fetch <path-to-bundle> 'refs/*:refs/recovered/*'
 git update-ref refs/heads/main refs/recovered/heads/main
```

如未来已获批准替换远端 refs，必须预先记录旧 SHA，并由管理员依据变更单逐个恢复；本文不提供 force push 或远端删除命令，也不执行这些动作。
