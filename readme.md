<h1 align="center">BlockScope</h1>

<p align="center">
  <img src="static/logo.svg" alt="BlockScope icon" width="128" height="128">
</p>

<p align="center">
  <a href="https://github.com/smiley-mcsmiles/BlockScope/releases"><img src="https://img.shields.io/badge/version-0.1.1-blue.svg?style=flat-square" alt="Version 0.1.1"></a>
  <a href="license.txt"><img src="https://img.shields.io/badge/license-BSD--3--Clause-green.svg?style=flat-square" alt="BSD 3-Clause License"></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/go-1.18%2B-00ADD8.svg?style=flat-square&logo=go&logoColor=white" alt="Go 1.18+"></a>
  <a href="Dockerfile"><img src="https://img.shields.io/badge/docker-ready-2496ED.svg?style=flat-square&logo=docker&logoColor=white" alt="Docker ready"></a>
  <a href="https://www.monero.org"><img src="https://img.shields.io/badge/network-Monero-FF6600.svg?style=flat-square&logo=monero&logoColor=white" alt="Monero network"></a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-00ADD8.svg?style=flat-square&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/JavaScript-F7DF1E.svg?style=flat-square&logo=javascript&logoColor=black" alt="JavaScript">
  <img src="https://img.shields.io/badge/HTML5-E34F26.svg?style=flat-square&logo=html5&logoColor=white" alt="HTML5">
  <img src="https://img.shields.io/badge/CSS3-1572B6.svg?style=flat-square&logo=css3&logoColor=white" alt="CSS3">
  <img src="https://img.shields.io/badge/Dockerfile-2496ED.svg?style=flat-square&logo=docker&logoColor=white" alt="Dockerfile">
</p>

<p align="center">
  A trustless <b>block explorer</b> for the Monero network that reads everything from <b>your own node</b>,<br>
  with a dark, hacker-green interface, a live mempool, and tools to decode outputs and prove payments.
</p>

BlockScope reads everything directly from your own `monerod` node, so you don't have to trust a third-party explorer for the chain data.

## Screenshots

**Blocks**

![Blocks page](docs/screenshots/blocks.png)

**Mempool**

![Mempool page](docs/screenshots/mempool-v2.png)

## Features

- **Network overview:** block height, last block age, hashrate, difficulty, mempool, fee rate and the most recent blocks. A "More stats" arrow reveals average block time, transactions and block size, the last block reward, mempool size and fees, peer counts and node status.
- **Block pages:** block details, reward, size and every transaction in the block, with previous and next block navigation.
- **Transaction pages:** confirmation status, block height, timestamp, size and double-spend detection.
- **Mempool:** unconfirmed transactions sorted by fee rate, with totals and a live search by transaction ID.
- **Decode outputs:** check which outputs in a transaction belong to a Monero address or subaddress and viewkey, and decode their amounts for RingCT transactions.
- **Prove sending:** prove that a transaction paid a given address, using the transaction private key from `get_tx_key`.
- **Light and dark themes**, plus keyboard shortcuts (`/` focuses search).

## Requirements

- Go 1.18 or newer, to build from source.
- A running Monero daemon (`monerod`) that BlockScope can reach. It must be reachable when BlockScope starts.

## Building

```
git clone https://github.com/smiley-mcsmiles/BlockScope
cd BlockScope
go build ./
```

This produces a `blockscope` binary in the project directory.

## Docker

Prebuilt images are published to GitHub Container Registry as `ghcr.io/smiley-mcsmiles/blockscope`. Pull the release version or `latest`, and run it on port `31312`:

```
docker run -p 31312:31312 ghcr.io/smiley-mcsmiles/blockscope:0.1.1 --daemon <node-host>:18081
podman run -p 31312:31312 ghcr.io/smiley-mcsmiles/blockscope:0.1.1 --daemon <node-host>:18081
```

To build the image yourself from this repository's source with the included `Dockerfile`:

```
docker build -t blockscope .
docker run -p 31312:31312 blockscope --daemon <node-host>:18081
```

Inside a container `127.0.0.1` refers to the container itself, so give `--daemon` the address of your node on the network.

## Running

Point BlockScope at your node with `--daemon`, then open the address given by `--bind` in your browser.

```
./blockscope --daemon 127.0.0.1:18081
```

Then open [http://127.0.0.1:31312](http://127.0.0.1:31312/).

### Flags

| Flag | Default | Description |
| --- | --- | --- |
| `--daemon` | `127.0.0.1:18081` | Address of the Monero daemon RPC. |
| `--bind` | `127.0.0.1:31312` | Address and port BlockScope listens on. |
| `--proxy` | `none` | Proxy for the daemon connection, such as `socks5://127.0.0.1:9050` for Tor. |
| `--rpc-login` | `none` | Credentials for a daemon with RPC login enabled, in the form `password:username`. |

### Remote and restricted nodes

To use a remote node, give its address to `--daemon`:

```
./blockscope --daemon <remote-node>:18081
```

To route the daemon connection through Tor:

```
./blockscope --proxy socks5://127.0.0.1:9050 --daemon <remote-node>:18081
```

The mempool page works best with the node's unrestricted RPC port (usually `18081`). Restricted RPC ports (usually `18089`) don't serve the full transaction pool, so BlockScope falls back to the calls they do allow. In that mode the mempool shows fees and sizes, but not when each transaction was received.

## Running as a service

The examples below run BlockScope headless, as its own unprivileged user, under your system's init system. Replace the placeholders before using them:

- `<node-ip>`: the address of your Monero node (use `127.0.0.1` if the node is on the same machine).
- `<bind-ip>`: the address BlockScope should listen on. Use `127.0.0.1` to keep it local, or the machine's LAN address to reach it from other devices.

These examples assume the binary is installed at `/usr/local/bin/blockscope`.

### 1. Create a headless user and install the binary

Give BlockScope its own account with no login shell and no home directory:

```sh
# Linux (Void, Debian, Ubuntu, Arch, Fedora)
sudo useradd --system --no-create-home --shell /usr/sbin/nologin blockscope

# Alpine
sudo adduser -S -D -H -s /sbin/nologin blockscope

# FreeBSD
sudo pw useradd blockscope -d /nonexistent -s /usr/sbin/nologin -c "BlockScope"

# OpenBSD
sudo useradd -d /var/empty -s /sbin/nologin -c "BlockScope" _blockscope
```

Install the binary and create a log directory owned by that user (use `_blockscope` as the owner on OpenBSD):

```sh
sudo install -m 0755 blockscope /usr/local/bin/blockscope
sudo install -d -o blockscope -g blockscope /var/log/blockscope
```

### 2. runit (Void Linux, Artix, and others)

Create `/etc/sv/blockscope/run`:

```sh
#!/bin/sh
exec 2>&1

LOG_DIR="/var/log/blockscope"
USER="blockscope"
GROUP="blockscope"

exec chpst -u $USER:$GROUP /usr/local/bin/blockscope \
    --daemon <node-ip>:18081 \
    --bind <bind-ip>:31312
```

Create `/etc/sv/blockscope/log/run` so the output goes to `/var/log/blockscope`:

```sh
#!/bin/sh
exec chpst -u blockscope:blockscope svlogd -tt /var/log/blockscope
```

Make both scripts executable, enable the service, and check it:

```sh
sudo chmod +x /etc/sv/blockscope/run /etc/sv/blockscope/log/run
sudo ln -s /etc/sv/blockscope /var/service/
sudo sv status blockscope
```

Common commands: `sudo sv restart blockscope`, `sudo sv stop blockscope`, `sudo sv start blockscope`. Use `/service` instead of `/var/service` on distributions that use that path.

### 3. systemd (Debian, Ubuntu, Fedora, Arch, and others)

Create `/etc/systemd/system/blockscope.service`:

```ini
[Unit]
Description=BlockScope Monero block explorer
After=network-online.target
Wants=network-online.target

[Service]
User=blockscope
Group=blockscope
ExecStart=/usr/local/bin/blockscope --daemon <node-ip>:18081 --bind <bind-ip>:31312
Restart=on-failure
RestartSec=5
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
```

Load the unit, enable it at boot, and start it now:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now blockscope
systemctl status blockscope
journalctl -u blockscope -f
```

### 4. OpenRC (Alpine, Gentoo, and others)

Create `/etc/init.d/blockscope`:

```sh
#!/sbin/openrc-run

name="BlockScope"
description="BlockScope Monero block explorer"
command="/usr/local/bin/blockscope"
command_args="--daemon <node-ip>:18081 --bind <bind-ip>:31312"
command_user="blockscope:blockscope"
command_background="yes"
pidfile="/run/${RC_SVCNAME}.pid"
supervisor="supervise-daemon"
output_log="/var/log/blockscope/blockscope.log"
error_log="/var/log/blockscope/blockscope.err"

depend() {
	need net
	after firewall
}
```

Make it executable, add it to the default runlevel, and start it:

```sh
sudo chmod +x /etc/init.d/blockscope
sudo rc-update add blockscope default
sudo rc-service blockscope start
rc-service blockscope status
```

### 5. FreeBSD (rc.d)

Create `/usr/local/etc/rc.d/blockscope`:

```sh
#!/bin/sh
#
# PROVIDE: blockscope
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name=blockscope
rcvar=blockscope_enable

load_rc_config $name
: ${blockscope_enable:=NO}
: ${blockscope_daemon:=<node-ip>:18081}
: ${blockscope_bind:=<bind-ip>:31312}

pidfile="/var/run/${name}.pid"
command="/usr/sbin/daemon"
command_args="-p ${pidfile} -u blockscope -o /var/log/blockscope/blockscope.log /usr/local/bin/blockscope --daemon ${blockscope_daemon} --bind ${blockscope_bind}"

run_rc_command "$1"
```

Make it executable, enable it, and start it:

```sh
sudo chmod +x /usr/local/etc/rc.d/blockscope
sudo sysrc blockscope_enable=YES
sudo service blockscope start
service blockscope status
```

### 6. OpenBSD (rc.d)

Create `/etc/rc.d/blockscope`:

```sh
#!/bin/ksh

daemon="/usr/local/bin/blockscope"
daemon_user="_blockscope"
daemon_flags="--daemon <node-ip>:18081 --bind <bind-ip>:31312"

. /etc/rc.d/rc.subr

rc_bg=YES

rc_cmd $1
```

Enable and start it with `rcctl`:

```sh
sudo chmod +x /etc/rc.d/blockscope
sudo rcctl enable blockscope
sudo rcctl start blockscope
rcctl check blockscope
```

> The FreeBSD and OpenBSD scripts, and the OpenRC script, are written against each system's standard rc framework but haven't been run on those systems yet. If one doesn't start, check its log file first.

Whichever init system you use, BlockScope needs the Monero node to be reachable when it starts. If the node is down, the service exits and the init system restarts it.

## Privacy

The decode outputs and prove sending tools send your Monero address and secret keys to the BlockScope server, which does the calculations. If you use a server you don't control, anyone running it can see those keys. Run BlockScope on your own machine or network when you use these tools.

## Development

```
go test ./
```

## Credits and license

BlockScope is derived from [MoneroBlock](https://github.com/duggavo/MoneroBlock) and is distributed under the BSD 3-Clause License. The original copyright notice is preserved in [license.txt](license.txt), and changes and additions are copyright WOOSAH. BlockScope is not endorsed by the original authors of MoneroBlock.

BlockScope also includes third-party components (BSD 3-Clause and Apache 2.0). Their license texts are in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).

## Donate

If BlockScope is useful to you, you can support its development:

| Coin | Address |
| --- | --- |
| Monero (XMR) | `82xtMVSmesuLjPtgHfBCEhM5Fpqh1SLLNf9pzHRRNPqQZsvrnmoM1ZGC7AiLyPfsufdyrMWHrWYV2hsC8jc5rEBVLMHWTLy` |
| Bitcoin (BTC) | `bc1qy2gtdhnfxp9dcs6v9jda748npmsjx3jgwp99mx` |
| CashApp | `$SmileyMcSmiles` |
