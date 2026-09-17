# zfs-pay

[English](README.md) | 简体中文

`zfs-pay` 用于关联 OpenZFS 叶级 vdev、Linux 块设备和服务器物理硬盘槽位。它为 Proxmox VE 和 Debian 管理员提供一条命令查看盘位、安全控制定位灯，并可通过 ZED 自动同步故障盘定位灯。

工具会从 ZFS、`lsblk`/udev、磁盘柜或阵列卡数据中动态发现映射，不需要维护手写盘位表。

> [!IMPORTANT]
> `zfs-pay` 唯一允许的硬件变更是打开或关闭定位灯。它不能将硬盘 offline、replace、rebuild、initialize、erase，也不能刷新固件或修改 ZFS pool、硬盘和 RAID 控制器配置。

## 功能

- 展示 ZFS pool、vdev 状态、Linux 设备、序列号、物理盘位、后端和 LED 状态。
- 支持紧凑的终端表格和带版本号的 JSON 输出。
- 可通过设备路径、序列号、ZFS GUID 或完整盘位 ID 精确定位一块硬盘。
- 身份缺失或匹配歧义时失败关闭，不猜测盘位。
- 支持 StorCLI 兼容控制器和 Linux SES；`ledctl` 需要显式启用。
- 通过 ZED 定位故障盘，并用已验证缓存保留已掉线硬盘最后一次确认的盘位。
- 自动化只关闭由 `zfs-pay` 自己点亮的定位灯。
- 提供可复现的 `amd64` 和 `arm64` Debian 安装包。

## 环境要求

- 使用 OpenZFS 的 Proxmox VE 或 Debian Linux。
- 安装软件和大多数 LED 操作需要 root 权限。
- 至少存在一个可用盘位后端：

| 后端 | 默认状态 | 说明 |
| --- | --- | --- |
| StorCLI 兼容 | 自动探测 | 推荐用于受支持的 Broadcom/LSI MegaRAID 控制器；StorCLI/PERCCLI 需要单独安装。 |
| Linux SES sysfs | 自动探测 | 需要内核提供块设备到 enclosure 的精确映射。 |
| `ledctl` | 默认关闭 | 仅在当前控制器和背板已经验证后启用。 |

StorCLI 和 PERCCLI 是专有工具，本项目及其安装包不会包含它们。没有可用 LED 后端时仍可安装，`status` 会报告能力缺失，而不是猜测盘位。

## 安装

下载与目标架构匹配的 `.deb`，并使用 `SHA256SUMS` 校验：

```sh
sha256sum -c SHA256SUMS --ignore-missing
apt install ./zfs-pay_VERSION_ARCH.deb
```

确认安装状态：

```sh
zfs-pay --version
zfs-pay status
systemctl is-enabled zfs-pay-reconcile.service
systemctl is-active zfs-zed
```

安装包会写入以下内容：

| 路径 | 用途 |
| --- | --- |
| `/usr/bin/zfs-pay` | 用户命令 |
| `/usr/lib/zfs-pay/zfs-pay-zed` | ZED/systemd 内部辅助程序 |
| `/etc/zfs/zed.d/*-zfs-pay.sh` | ZED hook 链接 |
| `/usr/lib/systemd/system/zfs-pay-reconcile.service` | 开机盘位对账服务 |
| `/etc/default/zfs-pay` | 自动化配置 |
| `/var/lib/zfs-pay/state.json` | 已验证盘位缓存和自动化 LED 所有权状态 |

安装或升级时会 reload systemd，并可能短暂重启 `zfs-zed` 以加载新 hook。它不会重启 ZFS pool 或虚拟机，也不会改变硬盘状态。

## 查看硬盘盘位

```sh
zfs-pay status
```

示例：

```text
POOL  STATE   DEVICE   SERIAL         BAY         BACKEND  LED
tank  ONLINE  /dev/sda TEST-SERIAL-A  c0/e10/s1  storcli  unknown
tank  ONLINE  /dev/sdb TEST-SERIAL-B  c0/e10/s2  storcli  unknown
```

盘位格式为 `controller/enclosure/slot`。例如 `c0/e10/s2` 表示控制器 0、背板或 enclosure 10、槽位 2。

需要查看完整 WWN 时，在 SERIAL 后增加一列，其他列保持不变：

```sh
zfs-pay status --wwn
```

SERIAL 是厂商序列号，WWN 是全球唯一的存储标识；缺失的 WWN 显示为 `-`。默认表格保持紧凑，使用 `--wwn` 时窄终端可能换行，但不会截断标识。JSON 已包含 WWN，`--wwn` 与 `--json` 同时使用不会改变 JSON 输出。

脚本和监控系统可以使用 JSON：

```sh
zfs-pay status --json
zfs-pay status --json | jq '.disks[] | {pool, state, vdev_guid, device_path, bay}'
```

JSON 当前使用 `schema_version: 1`。即使部分硬盘发现成功，全局和单盘诊断也会保留在输出中。

## 点亮硬盘定位灯

先运行 `zfs-pay status`（或 `status --wwn`），使用目标硬盘对应的 BAY 值。
下方 `c0/e10/s2` 只是示例，不是所有服务器通用的盘位。在一台服务器上首次操作前，建议先查看执行计划：

```sh
zfs-pay locate c0/e10/s2 --dry-run
```

点亮定位灯 60 秒。命令会等待，并在时间到达后自动关闭：

```sh
zfs-pay locate c0/e10/s2
```

需要到机箱前肉眼确认时，可以持续点亮 10 分钟：

```sh
zfs-pay locate c0/e10/s2 --timeout 10m
```

亮灯期间命令会一直在前台等待，这是正常行为，并非卡住，请保持终端会话开启。默认持续 60 秒，最长允许 24 小时。使用 `Ctrl+C` 中断命令时，也会在退出前尝试关闭定位灯。

需要提前关灯时，在原终端按 `Ctrl+C`，或在另一个终端执行：

```sh
zfs-pay locate c0/e10/s2 --off
```

`TARGET` 必须精确匹配一块硬盘：

```sh
zfs-pay locate /dev/sdb
zfs-pay locate /dev/sdb1
zfs-pay locate TEST-SERIAL-B
zfs-pay locate 7100000000000002
zfs-pay locate c0/e10/s2
```

以上示例依次是父磁盘、叶级设备、序列号、ZFS vdev GUID 和完整盘位 ID。如果某个值匹配多块硬盘，请改用 GUID 或完整盘位 ID。

### 拔盘前的确认

定位灯只用于识别实物，不会将硬盘从阵列分离或下线，亮灯不代表可以直接拔盘。永久移除 mirror 中的一块成员盘前，应先核对并记录硬盘身份和实际盘位，再通过 ZFS 管理工具分离目标成员，确认剩余 mirror 健康后才拔出。`zfs-pay` 不执行这些 ZFS 操作。分离后的硬盘已不属于存储池，可能无法再通过 `zfs-pay locate` 选中，因此应在分离前完成实物定位。

## 自动同步故障灯

Debian 安装包会启用 oneshot 对账服务，并为硬盘状态变化、pool import、vdev attach/clear 和系统启动安装 ZED hook。

对于磁盘 vdev，自动化处理以下状态：

| ZFS 状态 | 动作 |
| --- | --- |
| `DEGRADED`、`FAULTED`、`UNAVAIL`、`REMOVED` | 点亮已验证盘位的定位灯 |
| `ONLINE` | 仅当该灯由 `zfs-pay` 管理时关闭 |
| 其他状态或非磁盘 vdev | 安全忽略 |

确认当前 pool 健康后，可以手动执行一次对账：

```sh
zpool status -x
systemctl start zfs-pay-reconcile.service
systemctl status zfs-pay-reconcile.service --no-pager
```

查看自动化日志：

```sh
journalctl -u zfs-pay-reconcile.service
journalctl -u zfs-zed --since today
```

不要为了测试自动化而制造真实硬盘故障、offline 硬盘或拔盘。请使用 `locate --dry-run` 和项目内置夹具测试。

## 配置

自动化会读取 `/etc/default/zfs-pay`：

```sh
# 单次发现或后端命令的最长执行时间。
ZFS_PAY_COMMAND_TIMEOUT=20s

# 仅在硬件验证后显式启用。
# ZFS_PAY_ENABLE_LEDCTL=1
```

修改后执行一次对账，验证新配置：

```sh
systemctl start zfs-pay-reconcile.service
```

交互式命令需要使用同样的环境时，先加载配置：

```sh
set -a
. /etc/default/zfs-pay
set +a
zfs-pay status
```

## 排障

先执行只读检查：

```sh
zpool status -x
zfs-pay status
zfs-pay status --json
systemctl status zfs-zed --no-pager
systemctl status zfs-pay-reconcile.service --no-pager
journalctl -u zfs-pay-reconcile.service -n 50 --no-pager
```

常见诊断：

| 诊断 | 含义 |
| --- | --- |
| `unsupported` | 工具或盘位后端不可用；如果其他后端已经成功映射硬盘，这不是致命错误。 |
| `permission_denied` | 请使用所需权限运行，并检查设备或 sysfs 权限。 |
| `not_found` | 未找到目标硬盘或已验证的物理盘位。 |
| `ambiguous` | 匹配到多块硬盘或多个槽位，请使用精确 GUID 或完整盘位 ID。 |
| `timeout` | ZFS、Linux 或控制器命令超过了限定时间。 |
| `malformed_output` | 外部工具返回了无法安全解释的输出。 |

StorCLI 探测命令：

```sh
command -v storcli64
command -v storcli
test -x /opt/MegaRAID/storcli/storcli64 && echo "StorCLI found"
```

不要通过手写序列号到槽位的对照表绕过映射失败。应该修复身份数据或后端可见性，使硬盘更换或排序变化后仍可验证映射关系。

## 卸载

删除命令、服务和 ZED hook，但保留配置与盘位缓存：

```sh
apt remove zfs-pay
```

卸载前，安装包会尽力只关闭记录为由 `zfs-pay` 管理的定位灯。

同时删除配置和缓存状态：

```sh
apt purge zfs-pay
```

这两种操作都不会修改 ZFS pool 成员、vdev 状态、重建状态、固件或 RAID 配置。

## 从源码构建

项目使用 Go 1.24，目前没有第三方 Go module 依赖。

```sh
make verify
go test -race ./...
make build-linux
make package PACKAGE_VERSION=0.1.0~dev
make package-test
```

构建产物写入已忽略的 `dist/` 目录。Docker Buildx 会为 `amd64` 和 `arm64` 创建可复现的 Debian 安装包。

## 更多文档

- [CLI 契约](docs/cli.md)
- [安装细节](docs/install.md)
- [兼容性验证](docs/compatibility.md)
- [安全模型](docs/security.md)
- [发布流程](docs/release.md)
- [参与贡献](CONTRIBUTING.md)
- [安全问题报告](SECURITY.md)

## 许可证

MIT，详见 [LICENSE](LICENSE)。
