# iptv-vpn-proxy

Put all of your IPTV behind one VPN, on every device, without installing a VPN
on any of them.

> [!NOTE]
> **Changelog** · Last updated: November 14, 2026

<details>
<summary>Changelog</summary>

### November 14, 2026

- **There are now three containers instead of two.** A small helper, called
  the relay, has joined the VPN and nginx. It is a short Python program that
  runs in the official Python image with nothing installed on top, and its
  whole text sits inside `compose.yaml`, the same way the nginx settings do.
  You still fill in one file, `.env`, and nothing else.
- **Each provider needs one more line in `.env`:** `NAME_LIVE_TV=hls` or
  `NAME_LIVE_TV=ts`. It says which live format your players use for that
  provider, so set it to match what is chosen in the player. There is no
  default. A provider without this line is skipped, and the status page tells
  you which line to add.
  **Why it is needed:** it fixes a real fault. Many providers allow one
  session per channel on an account, and an MPEG-TS request and an HLS
  request for the same channel count as two. So when a recorder asked for
  MPEG-TS while a player was watching that channel on HLS, each new request
  ended the other one: the picture froze and recordings broke. The proxy can
  keep everyone on a channel on a single provider session, but only if it
  knows which format your players use, and it cannot work that out by itself.
- **New status page.** Open
  `http://<the machine's address>:<any provider's port>/status` in a web
  browser. It shows what is set up, what is wrong and how to fix it, where the
  VPN comes out, the state of each provider account, and what is playing. To
  show this it asks ipinfo.io (through the VPN) where the VPN comes out, and
  asks each provider for its account details, only while the page is open.
- **MPEG-TS: one connection per channel.** Several devices watching or
  recording the same channel now share one connection to the provider. A
  device that joins a channel that is already playing starts about 10 seconds
  behind live, so it has some video in hand if the provider pauses. When you
  change channel, the old connection is let go before the new one is opened.
- **HLS: watch and record a channel at once.** A recorder that asks for
  MPEG-TS is now fed from the channel's HLS session, the one your players are
  already on. A recorder and a viewer on one channel no longer knock each
  other off.
- **HLS: the proxy lets go of the provider sooner when you stop.** The proxy
  keeps its connection to a provider's video server open for a while after
  the last request, so the next one can reuse it. That time is now 20 seconds;
  it was 55. This is only the proxy's side. How long the provider itself goes
  on counting you as connected after you stop is up to the provider and is
  usually longer; no fixed figure is claimed for it here. Connections are
  still reused while a channel plays.
- **nginx is still plain nginx.** It is the stock image with settings only and
  no add-on scripting. Everything new that needed a program lives in the
  relay. If the relay is not running, TV still plays, without the sharing and
  without the status page.
- **Safety.** The proxy is stricter about refusing addresses that point at the
  machine itself or at your home network: it catches another way of writing
  such an address, and the relay checks where a provider's redirect really leads
  before it connects. The notes nginx and the relay pass to each other are
  never accepted from a player or from a provider's server, so a server
  cannot use them to steer your player outside the VPN.
- **If you are updating:** replace `compose.yaml`, add the `NAME_LIVE_TV` line
  to every provider in `.env`, then run
  `docker compose up -d --force-recreate`. Until a provider has that line it
  is skipped, and with none left nothing plays; the status page says which
  line is missing. A provider can no longer use port 8197, which the relay
  now uses inside the stack. A change to `.env` now restarts the relay along
  with nginx, which ends any shared MPEG-TS stream for a moment.

</details>

> [!TIP]
> **There is a status page.** Open
> `http://<the machine's address>:<any provider's port>/status` in a web
> browser, for example `http://192.168.1.50:8095/status`. It shows what is set
> up, what is wrong and the exact line to fix, where the VPN comes out, the
> state of each provider account, and what is playing. Look there first when
> something does not work.

Three containers run on a machine at home. Your players connect to that
machine instead of to your IPTV provider, and everything they fetch leaves
through the VPN. On the phone, TV or streaming box you type an address and a
login into your player, the same way you would with the provider's own
details. That is the whole setup on the device.

- **gluetun** holds the VPN tunnel.
- **nginx** sits inside that tunnel and passes your players' requests on to the provider.
- **the relay**, a small Python helper, sits inside the tunnel too. When
  several devices watch or record the same live channel, it lets them share
  one connection to the provider. It also draws the status page.

> **For your home network only.** The proxy ports have no protection of their
> own. Never forward them from the internet. To watch away from home, reach
> your home network privately (Tailscale, your own WireGuard, and so on).

Use it only with services you are entitled to use.

## Contents

- [Design goals](#design-goals)
- [Why this exists](#why-this-exists)
- [What you need](#what-you-need)
- [Set it up](#set-it-up)
- [The two files](#the-two-files)
- [Everyday use](#everyday-use)
- [With a playlist manager](#with-a-playlist-manager)
- [How it works](#how-it-works)
- [What was tested, and how](#what-was-tested-and-how)
- [Trade-offs and things to know](#trade-offs-and-things-to-know)
- [What it stores, updating, removing](#what-it-stores-updating-removing)
- [Extras that need one more file](#extras-that-need-one-more-file)
- [Platform notes](#platform-notes)
- [When something is wrong](#when-something-is-wrong)
- [How it works (technical)](#how-it-works-technical)
- [Support](#support)
- [License](#license)

## Design goals

1. **Parity with direct credentials.** A player pointed at the proxy should
   behave exactly as if you had typed the provider's own Xtream login into it.
   Not better, not different: the same, with the VPN added.
2. **One VPN for everything.** Every device and every player goes out through
   the same tunnel, and no device needs a VPN app.
3. **The easiest setup that can work.** One settings file. You never edit the
   compose file. Adding a provider is five lines.
4. **Works with any player and any Xtream provider.** Nothing in it is written
   for one provider or one app.
5. **Nothing to build or install.** It is stock gluetun, stock nginx and the
   stock Python image, with nothing added to any of them. Everything that
   makes them a proxy is text inside the compose file, where you can read it:
   the nginx settings, a few lines of shell that read your settings at start
   and keep a list of the video servers your streams come from, and one
   Python program (the relay) that uses only what Python ships with.

## Why this exists

A plain reverse proxy in front of an IPTV provider usually works for MPEG-TS
and then freezes every few seconds on HLS. The cause:

- Many providers start a **new session** every time a channel's address is asked for.
- A new session **ends the previous one** on that channel.
- An HLS player asks for that address again every few seconds. A second device
  or a recording on the same channel asks too.

A proxy that makes a fresh trip to the provider for each of those requests
keeps ending its own session, so the pieces the player was about to download
stop existing. A player connected directly does not have this problem, because
it follows the provider's redirect once and then stays on that session.

This proxy does what the direct player does. It remembers where the provider
sent each channel and keeps every reader of that channel on that one session.

MPEG-TS has the same problem as soon as a second device or a recorder opens a
channel that is already playing. With a provider that allows one connection
per channel, the readers end each other over and over. So the relay opens each
live MPEG-TS channel at the provider once and gives every reader of that
channel the same video.

## What you need

- A machine at home that is always on, with Docker and Docker Compose 2.24 or newer.
- A VPN that gluetun supports, or a WireGuard or OpenVPN config file from any VPN.
- IPTV providers that give you an **Xtream login**: a server address, a username and a password.

## Set it up

1. Make a folder and put `compose.yaml` and `.env.example` in it. Both are in
   this repository.
2. Copy `.env.example` to `.env`.
3. Fill in `.env`. It has four sections and explains each one:
   your VPN, the easy login you choose, the port range, and your providers.
   Each provider is five lines. The fifth, `NAME_LIVE_TV`, is `hls` or `ts`:
   whichever live format is chosen in your player for that provider.
4. Start it:
   ```
   docker compose up -d
   ```
5. Check that it works. Open the status page in a web browser:
   ```
   http://<the machine's address>:<any provider's port>/status
   ```
   It says whether the proxy is working, lists each provider, and names
   anything that is wrong along with the line to fix. The log of the
   `iptv-nginx` container says the same in short at each start:
   ```
   PROVIDER PROVIDER1 on port 8095 -> https://provider.example
   STATUS PAGE: open http://<this machine's address>:8095/status in a web browser
   ```
   A provider with a problem is listed as `SKIPPED` with the reason, and the
   others still start. Each of the three containers keeps a log:
   ```
   docker logs iptv-nginx
   docker logs iptv-relay
   docker logs iptv-vpn
   ```
6. In your player or playlist manager, add an **Xtream** source:
   - server: `http://<the machine's address>:<that provider's port>`
   - username and password: the `PLAYER_USER` and `PLAYER_PASSWORD` you chose

Each provider gets its own port. The same easy login works for all of them.

## The two files

These are the same files that sit in this repository, shown here in full so
you can read and copy them without leaving the page. Click a heading to open
it.

### `.env.example`: every setting, with instructions

Copy it to `.env` and fill it in. This is the only file you edit. It covers the
VPN, the login your players use, the ports, and one block of five lines per
provider.

<details>
<summary><b>Show .env.example</b></summary>

```sh
##############################################################################
# Settings for the IPTV VPN proxy. Copy this file to ".env" and fill it in.
#
# This one file holds everything:
#   1. your VPN (the tunnel everything goes out through)
#   2. the one easy login your apps use with the proxy
#   3. the ports the proxy may use
#   4. one block of five lines per IPTV provider
#
# You never edit compose.yaml. Change this file, then bring the stack up
# again ("docker compose up -d", or the Up / Deploy / Build button of your
# Docker manager). Compose notices the change and restarts what needs it:
#   - a change to a provider or to the easy login restarts only the proxy
#     and its helper (the relay); the VPN stays up
#   - a change to a VPN setting or to the port range also restarts the tunnel,
#     which interrupts whatever is playing for a few seconds
#
# Lines starting with # are notes and are ignored.
# Write values plainly after the = sign, with no spaces around the =.
# A value containing anything besides letters, digits and  . - _ : / ,
# goes between single quotes:   SOMETHING='p@ss w#rd$1'
##############################################################################

# Your time zone, for the times in the logs (e.g. America/New_York, Europe/London).
TZ=Etc/UTC


##############################################################################
# 1. THE VPN
#
# The tunnel is made by gluetun, and every line in this section is one of
# gluetun's own settings, described in its documentation:
#     https://github.com/qdm12/gluetun-wiki   (look up your VPN provider
#     under "setup/providers" for the exact lines it needs)
# The settings shown in this section are most of the ones this stack passes on
# to gluetun (SERVER_REGIONS and SERVER_HOSTNAMES are passed on too). The
# README says how to add a different one.
#
# Pick ONE of the four ways below, remove the # from its lines and fill them
# in. Leave the others commented out.
##############################################################################

# ---- Way 1: WireGuard, a VPN provider gluetun knows by name ---------------
# The name is your VPN company in lower case, for example mullvad, protonvpn,
# nordvpn, surfshark, airvpn or ivpn. WIREGUARD_ADDRESSES is the
# "Address" line of the WireGuard file your provider gives you. The preshared
# key is only for providers that give one. The last two lines are optional
# and choose where you exit.
# VPN_SERVICE_PROVIDER=mullvad
# VPN_TYPE=wireguard
# WIREGUARD_PRIVATE_KEY=
# WIREGUARD_ADDRESSES=10.64.222.21/32
# WIREGUARD_PRESHARED_KEY=
# SERVER_COUNTRIES=Netherlands
# SERVER_CITIES=Amsterdam

# ---- Way 2: WireGuard, from a .conf file of any provider or your own server
# Copy each value from the .conf file:
#   WIREGUARD_PRIVATE_KEY     "PrivateKey" under [Interface]
#   WIREGUARD_ADDRESSES       "Address" under [Interface]
#   WIREGUARD_PUBLIC_KEY      "PublicKey" under [Peer]
#   WIREGUARD_PRESHARED_KEY   "PresharedKey" under [Peer], if there is one
#   WIREGUARD_ENDPOINT_IP     "Endpoint", the part before the colon
#   WIREGUARD_ENDPOINT_PORT   "Endpoint", the part after the colon
# VPN_SERVICE_PROVIDER=custom
# VPN_TYPE=wireguard
# WIREGUARD_PRIVATE_KEY=
# WIREGUARD_ADDRESSES=10.0.0.2/32
# WIREGUARD_PUBLIC_KEY=
# WIREGUARD_PRESHARED_KEY=
# WIREGUARD_ENDPOINT_IP=203.0.113.10
# WIREGUARD_ENDPOINT_PORT=51820

# ---- Way 3: OpenVPN, a VPN provider gluetun knows by name ------------------
# For example nordvpn, expressvpn, cyberghost or surfshark. The protocol is
# udp (faster) or tcp (gets through more networks). SERVER_COUNTRIES is
# optional.
# VPN_SERVICE_PROVIDER=nordvpn
# VPN_TYPE=openvpn
# OPENVPN_USER=
# OPENVPN_PASSWORD=
# OPENVPN_PROTOCOL=udp
# SERVER_COUNTRIES=Netherlands

# ---- Way 4: OpenVPN, from an .ovpn file ------------------------------------
# gluetun has to be handed the file itself. That takes one small extra file
# next to compose.yaml; the README shows it ("Extras that need one more
# file"). The login lines are only needed if the file asks for one.
# VPN_SERVICE_PROVIDER=custom
# VPN_TYPE=openvpn
# OPENVPN_CUSTOM_CONFIG=/gluetun/custom.ovpn
# OPENVPN_USER=
# OPENVPN_PASSWORD=

# ---- Sometimes needed with WireGuard ---------------------------------------
# Lower the MTU if streams stall or pages half-load (try 1320, then 1280).
# WIREGUARD_MTU=1320
# Keep the tunnel awake if it drops when idle.
# WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL=25s
# Use this if the machine's kernel has no WireGuard (some NAS models, WSL 2).
# WIREGUARD_IMPLEMENTATION=userspace

# ---- Optional, for any of the four ways -----------------------------------
# Your players reach the proxy without any setting here.
#
# FIREWALL_OUTBOUND_SUBNETS is for the other direction: networks the
# containers may connect OUT to directly, outside the tunnel. Leave it out
# unless you know you need it.
# FIREWALL_OUTBOUND_SUBNETS=192.168.1.0/24
#
# Which DNS the tunnel uses. By default gluetun uses encrypted DNS and blocks
# known-malicious names; leave these out unless you know you need them.
#
# To use one particular DNS server instead (your VPN provider's own, for
# example), set the two lines below. THE ADDRESS MUST END IN :53, the DNS
# port. Without the port gluetun does not accept the address and nothing can
# look up names. For several servers, separate them with commas.
#     DNS_UPSTREAM_RESOLVER_TYPE=plain
#     DNS_UPSTREAM_PLAIN_ADDRESSES=10.0.0.1:53
# (for example: DNS_UPSTREAM_RESOLVERS=quad9   or   BLOCK_MALICIOUS=off)


##############################################################################
# 2. THE LOGIN THE PROXY ANSWERS TO
#
# This is the easy username and password you type into your player or
# playlist manager, for every provider. The proxy swaps it for that
# provider's real login on the way out, so the real logins are typed in one
# place only: this file. The same easy login works for every provider; the
# PORT picks the provider.
# You choose these two. Use letters, digits, - and _ only.
##############################################################################
PLAYER_USER=choose-a-name
PLAYER_PASSWORD=choose-a-password


##############################################################################
# 3. THE PORTS THE PROXY MAY USE, as a range: FIRST-LAST
#
# Every provider below needs its own port inside this range. Set it to
# whatever ports are free on your machine: 8095-8096 is room for two
# providers, 8095-8099 for five. Widen or narrow it any time and bring the
# stack up again (that one change also restarts the VPN).
##############################################################################
PROXY_PORTS=8095-8096


##############################################################################
# 4. YOUR PROVIDERS
#
# Each provider is a block of five lines that share one NAME:
#
#     NAME_URL=https://provider.example      the server address the provider
#                                            gave you: http:// or https://,
#                                            the server name, optionally
#                                            :port, and nothing after that
#     NAME_PORT=8095                         a free port inside PROXY_PORTS
#     NAME_USER=the-username-they-gave-you
#     NAME_PASSWORD=the-password-they-gave-you
#     NAME_LIVE_TV=hls                       how your players watch its live
#                                            channels: hls or ts (see below)
#
# NAME is any word you like in CAPITALS (letters, digits, _), for example
# PROVIDER1 or SPORTS. It is only a label; all five lines must use the same
# one.
#
# NAME_LIVE_TV: HLS OR MPEG-TS. Say which live format your players use for
# this provider. There is no default, and a provider without this line is
# left out.
#     hls   Your players use HLS. Everyone watching a channel shares one
#           connection to the provider. A recorder that asks for MPEG-TS is
#           given the same video as your players, so you can watch and
#           record a channel at once.
#     ts    Your players use MPEG-TS. Everyone on a channel shares one
#           connection to the provider and gets the provider's own stream,
#           so you can watch and record a channel at once. Do not open a
#           channel as HLS while it is playing or recording as MPEG-TS:
#           most providers allow one kind at a time per channel.
# Use the format chosen in your players, and in your playlist manager if you
# have one. They all have to agree. If a provider offers only one of the
# two, use that one.
#
# PROVIDER LOGINS CAN CONTAIN ANY CHARACTERS. You do not get to choose them,
# so the proxy takes whatever the seller gave you. One rule for THIS FILE: if
# a username or password has anything in it besides letters and digits, put
# it between single quotes, exactly as given:
#        NAME_PASSWORD='p@ss w#rd$1%'
# Without the quotes, a $ or a # in the value is read as something else and
# part of the login is silently lost. If the login itself contains a single
# quote, write that one character as \'  (backslash, then the quote).
#
# TO ADD A PROVIDER
#   1. Copy the example block below, remove the # in front of its five
#      lines, change NAME, and fill in the values.
#   2. Bring the stack up again.
#   3. Check it was picked up: the iptv-nginx container's log lists the
#      providers found at every start, like
#          PROVIDER PROVIDER1 on port 8095 -> https://provider.example
#      A provider with a problem is listed as SKIPPED with the reason, and
#      the other providers keep working.
#      The status page shows the same and more. Open it in a web browser:
#          http://<this machine's address>:<any provider's port>/status
#   4. In your player or playlist manager add an Xtream source with
#          server    http://<this machine's address>:<NAME_PORT>
#          username  the PLAYER_USER above
#          password  the PLAYER_PASSWORD above
#
# TO REMOVE A PROVIDER: delete its five lines (or put # in front of them)
# and bring the stack up again.
#
# A provider's port is what tells the proxy which provider you mean, so if
# you change a port, change it in your apps too.
##############################################################################

# ---------- example (remove the # signs to use it) ----------
# PROVIDER1_URL=https://provider.example
# PROVIDER1_PORT=8095
# PROVIDER1_USER=the-username-they-gave-you
# PROVIDER1_PASSWORD=the-password-they-gave-you
# PROVIDER1_LIVE_TV=hls

# ---------- a second one ----------
# PROVIDER2_URL=http://another-provider.example:8080
# PROVIDER2_PORT=8096
# PROVIDER2_USER=the-username-they-gave-you
# PROVIDER2_PASSWORD='the-p@ssword-they-gave-you'
# PROVIDER2_LIVE_TV=hls

```

</details>

### `compose.yaml`: the stack itself

You do not edit this one. It is in this repository as
[compose.yaml](compose.yaml) and shown in full below. It defines the three
containers and holds, as plain text with a comment on each part saying what it
is for:

- the whole nginx configuration
- the short shell script that reads your settings when nginx starts
- the relay, a Python program of about 1,000 lines that uses only what Python
  ships with

Nothing is downloaded or installed when the stack starts except the three
stock images.

<details>
<summary><b>Show compose.yaml</b></summary>

```yaml
##############################################################################
# IPTV through a VPN, with nothing to install on your devices
#
#   iptv-vpn     gluetun. Holds the VPN tunnel (WireGuard or OpenVPN, any
#                provider gluetun supports).
#   iptv-nginx   A reverse proxy in front of your IPTV providers. It lives
#                inside the tunnel's network, so everything it fetches from a
#                provider goes out through the VPN and nothing else can.
#   iptv-relay   A small helper beside nginx, also inside the tunnel's
#                network. When several of your devices watch or record the
#                same live channel, it lets them share one connection to
#                the provider. It also draws the status page. It is plain
#                Python in the stock Python image; its whole text is at the
#                bottom of this file.
#
# Your players and playlist managers connect to this machine instead of to
# the provider:
#        http://<this machine>:<the provider's port>
#        username / password = PLAYER_USER / PLAYER_PASSWORD from .env
# and behave as if the provider's own Xtream login had been typed into them.
#
# YOU NEVER EDIT THIS FILE. Every setting is in .env (copy .env.example to
# .env and fill it in). To add a provider, add five lines to .env:
#        NAME_URL=https://provider.example
#        NAME_PORT=8095
#        NAME_USER=...
#        NAME_PASSWORD=...
#        NAME_LIVE_TV=hls          (or ts: how your players watch its live TV)
# then bring the stack up again. The iptv-nginx log lists the providers it
# found at each start, and names any it skipped and why.
#
# What the proxy does besides passing bytes on (each part is explained where
# it sits in the config at the bottom of this file):
#   1. keeps redirects inside the tunnel
#   2. refuses to be pointed at this machine or your home network
#   3. points the pieces of an HLS playlist back at itself
#   4. keeps ONE provider session per live HLS channel
#   5. swaps the easy login for the provider's
#   6. keeps ONE provider connection per live channel for MPEG-TS readers
#      and recorders too (done by the small "relay" beside nginx)
#
# To see what is set up, and what is wrong when something is, open
#        http://<this machine>:<any provider's port>/status
# in a web browser. The iptv-nginx log prints that address at every start.
#
# Each container keeps a log of what it is doing:
#        docker logs iptv-vpn        the VPN tunnel
#        docker logs iptv-nginx      the providers it found, and every request
#        docker logs iptv-relay      each live channel it opens and closes,
#                                    and anything that went wrong with one
#
# Works with HLS, MPEG-TS, movies, series, catch-up and the guide.
#
# FOR YOUR HOME NETWORK ONLY. The proxy ports have no protection of their
# own. Never forward them from the internet; reach them from outside with
# your own private network (Tailscale, WireGuard, ...) instead.
#
# Good to know:
#   - Xtream logins only. A provider's ready-made M3U link (get.php) is
#     refused, because the addresses inside it skip the VPN.
#   - When a player logs in, the provider's reply includes the provider's own
#     server address, and the proxy passes that reply on unchanged. A player
#     that connects to that address instead of the one you typed would skip
#     the VPN.
#   - Logos and posters named in the provider's lists are pointed back at
#     the proxy, so they come through the VPN too. Pictures named inside the
#     guide file are not; the player fetches those directly.
##############################################################################

services:

  # ---------------------------------------------------------------------------
  # The VPN tunnel. Anything sharing this container's network has no other way
  # out, so if the tunnel is down nothing leaks. All of its settings are
  # gluetun's own, read from .env (see the notes there).
  # ---------------------------------------------------------------------------
  vpn:
    image: qmcgaw/gluetun:latest
    container_name: iptv-vpn
    restart: unless-stopped
    cap_add:
      - NET_ADMIN
    devices:
      - /dev/net/tun:/dev/net/tun
    ports:
      # Every provider port, same number inside and out. One range covers all
      # providers, present and future. The range is PROXY_PORTS in .env: set it
      # to whatever ports are free on your machine.
      - "${PROXY_PORTS:-8095-8096}:${PROXY_PORTS:-8095-8096}"
    # The VPN settings, taken from .env by name. A setting you leave out of
    # .env is simply not passed on, and gluetun uses its own default. Only a
    # change to one of these (or to PROXY_PORTS) restarts the tunnel; adding or
    # changing an IPTV provider does not.
    environment:
      - TZ
      - VPN_SERVICE_PROVIDER
      - VPN_TYPE
      - SERVER_COUNTRIES
      - SERVER_REGIONS
      - SERVER_CITIES
      - SERVER_HOSTNAMES
      - WIREGUARD_PRIVATE_KEY
      - WIREGUARD_ADDRESSES
      - WIREGUARD_PRESHARED_KEY
      - WIREGUARD_PUBLIC_KEY
      - WIREGUARD_ENDPOINT_IP
      - WIREGUARD_ENDPOINT_PORT
      - WIREGUARD_MTU
      - WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL
      - WIREGUARD_IMPLEMENTATION
      - OPENVPN_USER
      - OPENVPN_PASSWORD
      - OPENVPN_PROTOCOL
      - OPENVPN_CUSTOM_CONFIG
      - FIREWALL_OUTBOUND_SUBNETS
      - DNS_UPSTREAM_RESOLVER_TYPE
      - DNS_UPSTREAM_RESOLVERS
      - DNS_UPSTREAM_PLAIN_ADDRESSES
      - BLOCK_MALICIOUS
      # gluetun's own control page stays inside the container.
      - HTTP_CONTROL_SERVER_ADDRESS=127.0.0.1:8000

  # ---------------------------------------------------------------------------
  # nginx. Shares the tunnel's network. Its whole configuration is the text
  # under "configs:" at the bottom of this file.
  # ---------------------------------------------------------------------------
  nginx:
    image: nginx:alpine
    container_name: iptv-nginx
    restart: unless-stopped
    network_mode: "service:vpn"         # all its traffic goes through the tunnel
    depends_on:
      vpn:
        condition: service_healthy
        restart: true                   # comes back with the tunnel
    # The provider lines are read from your settings file. That file is .env;
    # Portainer saves the one you upload as stack.env. Whichever exists is used.
    env_file:
      - path: .env
        required: false
      - path: stack.env
        required: false
    volumes:
      # Where the proxy remembers each channel's session. Kept in a small Docker
      # volume so that when this container is recreated (a provider was added,
      # an update), channels that are playing carry on with the same session.
      - hls-memory:/var/cache/nginx/hls
      # The video servers the proxy has learned (see "entrypoint" below).
      - video-servers:/var/lib/video-servers
    configs:
      - source: proxy
        target: /etc/nginx/conf.d/default.conf
      - source: provider-locations
        target: /etc/nginx/provider-locations.conf
    # At every start: build one nginx server block per provider found in .env
    # (any NAME with NAME_URL, NAME_PORT, NAME_USER, NAME_PASSWORD and
    # NAME_LIVE_TV), then
    # start nginx as usual. A provider with a missing or unusable line is
    # skipped and named in the log; the others still start.
    #
    # Provider logins may contain ANY characters. Each one is percent-encoded
    # here (letters, digits and - . _ ~ stay as they are; everything else
    # becomes %XX), which is the form a URL needs, so nginx only ever sees
    # safe text and the provider receives exactly the login it issued.
    entrypoint:
      - /bin/sh
      - -c
      - |
        # The status page is drawn by the relay. nginx hands it the request, and
        # lets it read this start's notes about what was found (written below).
        cat > /etc/nginx/status.inc <<'STATUS'
        location = /status {
          proxy_intercept_errors off;
          proxy_read_timeout 30s;
          proxy_set_header Connection "";
          proxy_set_header Host $$http_host;
          proxy_set_header X-Hls-Provider "";
          proxy_set_header X-Hls-Renew "";
          proxy_set_header X-Relay-Port $$server_port;
          proxy_set_header X-Relay-User $$easy_user;
          proxy_set_header X-Relay-Pass $$easy_pass;
          proxy_set_header X-Relay-Provider "";
          proxy_set_header X-Relay-Uri "";
          proxy_set_header X-Relay-Orig "";
          error_page 502 504 = @status_down;
          proxy_pass http://127.0.0.1:8197/status;
        }
        location = /__status.json { allow 127.0.0.1; deny all; access_log off; default_type application/json; alias /etc/nginx/status.json; }
        location @status_down { default_type text/plain; return 503 "The status page is drawn by the iptv-relay container, and it is not answering. Look at its log: docker logs iptv-relay\n"; }
        STATUS
        out=/etc/nginx/conf.d/providers.conf
        : > "$$out"
        enc() {
          printf '%s' "$$1" | hexdump -v -e '1/1 "%02x\n"' | while read -r h; do
            case "$$h" in
              2d|2e|5f|7e|3[0-9]|4[1-9a-f]|5[0-9a]|6[1-9a-f]|7[0-9a]) printf "\\x$$h" ;;
              *) printf '%%%s' "$$(echo "$$h" | tr a-f A-F)" ;;
            esac
          done
        }
        ports="$${PROXY_PORTS:-8095-8096}"; stop=""
        # Notes for the status page (/status): what was found, what was left out and why.
        notes=/tmp/status-notes; rm -rf "$$notes"; mkdir -p "$$notes"; : > "$$notes/problems"; : > "$$notes/providers"
        clean() { printf '%s' "$$1" | tr -d '"\\' | tr '\n\r\t' '   '; }
        echo "$$ports" | grep -Eqx '[1-9][0-9]{0,4}(-[1-9][0-9]{0,4})?' || stop="PROXY_PORTS must be one port or a range like 8095-8099 (found: $$ports)"
        [ -z "$$stop" ] && [ "$$(printf '%s' "$$PLAYER_USER$$PLAYER_PASSWORD$$ports" | wc -l)" != 0 ] && stop="PLAYER_USER, PLAYER_PASSWORD and PROXY_PORTS must each be on one line"
        [ -z "$$stop" ] && { echo "$$PLAYER_USER" | grep -Eqx '[A-Za-z0-9_-]+' && echo "$$PLAYER_PASSWORD" | grep -Eqx '[A-Za-z0-9_-]+' || stop="PLAYER_USER and PLAYER_PASSWORD must be letters, digits, - or _ only (they are yours to choose)"; }
        [ -z "$$stop" ] && case "$$PLAYER_USER" in live|movie|series|timeshift) stop="PLAYER_USER cannot be live, movie, series or timeshift";; esac
        lo=$${ports%-*}; hi=$${ports#*-}; used=" "
        [ -n "$$stop" ] && { echo "PROXY SETTINGS PROBLEM: $$stop"; printf '"%s."\n' "$$(clean "$$stop")" >> "$$notes/problems"; }
        env | sed -n 's/^\([A-Za-z0-9_]*[a-z][A-Za-z0-9_]*\)_URL=.*/\1/p' | while read -r low; do echo "PROVIDER $$low SKIPPED: the name must be in CAPITALS"; printf '{"name":"%s","port":"","ok":false,"why":"the name must be in CAPITALS"}\n' "$$(clean "$$low")" >> "$$notes/providers"; done
        [ -z "$$stop" ] && for name in $$(env | sed -n 's/^\([A-Z][A-Z0-9_]*\)_URL=.*/\1/p' | sort); do
          url=$$(printenv "$${name}_URL"); port=$$(printenv "$${name}_PORT")
          user=$$(printenv "$${name}_USER"); pass=$$(printenv "$${name}_PASSWORD")
          bad=""
          case "$$name" in PROXY|PLAYER|VPN|OPENVPN|WIREGUARD|SERVER|FIREWALL|DNS) bad="$$name is one of the proxy's own names; pick another";; esac
          [ -z "$$bad" ] && { [ -n "$$url" ] && [ -n "$$port" ] && [ -n "$$user" ] && [ -n "$$pass" ] || bad="needs $${name}_URL, _PORT, _USER and _PASSWORD"; }
          [ -z "$$bad" ] && { echo "$$port" | grep -Eqx '[1-9][0-9]{0,4}' && [ "$$port" -le 65535 ] || bad="port must be a number from 1 to 65535"; }
          [ -z "$$bad" ] && { echo "$$url" | grep -Eqx 'https?://[A-Za-z0-9._-]+(:[0-9]{1,5})?/?' || bad="address must be http://name or https://name, optionally :port, with nothing after it"; }
          [ -z "$$bad" ] && case " 53 8000 8195 8197 9999 " in *" $$port "*) bad="port $$port is used inside the VPN container itself";; esac
          [ -z "$$bad" ] && case "$$used" in *" $$port "*) bad="port $$port is already used by another provider";; esac
          [ -z "$$bad" ] && { [ "$$port" -ge "$$lo" ] && [ "$$port" -le "$$hi" ] || bad="port $$port is outside PROXY_PORTS ($$ports)"; }
          mode=$$(printenv "$${name}_LIVE_TV")
          [ -z "$$bad" ] && [ "$$(printf '%s' "$$url$$port$$user$$pass$$mode" | wc -l)" != 0 ] && bad="one of its lines is broken over two lines"
          [ -z "$$bad" ] && case "$$mode" in hls|ts) ;; *) bad="needs the line $${name}_LIVE_TV=hls if your players use HLS, or $${name}_LIVE_TV=ts if they use MPEG-TS";; esac
          if [ -n "$$bad" ]; then echo "PROVIDER $$name SKIPPED: $$bad"; printf '{"name":"%s","port":"","ok":false,"why":"%s"}\n' "$$name" "$$(clean "$$bad")" >> "$$notes/providers"; continue; fi
          used="$$used$$port "
          printf 'server {\n  listen %s;\n  set $$provider  %s;\n  set $$pscheme   %s;\n  set $$acct_user "%s";\n  set $$acct_pass "%s";\n  set $$live_tv   %s;\n  include /etc/nginx/provider-locations.conf;\n}\n' \
            "$$port" "$${url%/}" "$${url%%://*}" "$$(enc "$$user")" "$$(enc "$$pass")" "$$mode" >> "$$out"
          echo "PROVIDER $$name on port $$port -> $${url%/}"
          printf '{"name":"%s","port":"%s","ok":true,"mode":"%s"}\n' "$$name" "$$port" "$$mode" >> "$$notes/providers"
          { [ -z "$$first" ] || [ "$$port" -lt "$$first" ]; } && first="$$port"
        done
        [ "$$used" = " " ] && echo "NO PROVIDERS STARTED. nginx is running with none: fix .env and bring the stack up again."
        # The status page. With no provider running, one port still answers so the
        # page can say what is wrong, and the container reports itself unhealthy.
        : > /var/run/proxy-problem
        if [ "$$used" = " " ]; then
          echo "not set up" > /var/run/proxy-problem
          first=$$(echo "$$lo" | grep -Ex '[1-9][0-9]{0,4}' || echo 8095)
          printf 'server {\n  listen %s;\n  set $$easy_user "";\n  set $$easy_pass "";\n  include /etc/nginx/status.inc;\n  location / { default_type text/plain; return 503 "The IPTV proxy is not set up yet. Open /status on this address in a web browser to see what is missing.\\n"; }\n}\n' "$$first" >> "$$out"
        fi
        printf '{"problems":[%s],"providers":[%s]}\n' "$$(paste -sd, "$$notes/problems")" "$$(paste -sd, "$$notes/providers")" > /etc/nginx/status.json
        rm -rf "$$notes"
        echo "STATUS PAGE: open http://<this machine's address>:$$first/status in a web browser"
        # Keep connections to the provider's video servers open, the way a player
        # talking to them directly does. nginx only reuses connections to servers
        # it has a line for, and no server name is written in this file: nginx
        # notes every video server a stream actually comes from (see "video
        # servers" in the config below) and each new name gets a line of its own.
        # The list is kept in a small Docker volume, so after the first time a
        # server is seen it is already known at every later start and nothing has
        # to be re-read while someone is watching.
        # "max_fails=0": a server is never set aside after a failed request; the
        # next request simply tries it again, as it would with no line for it.
        # A provider can move to new servers at any time. A server that has never
        # been seen works the old way (a new connection for every request) until
        # the loop below adds it: it waits for a run of channel changes to end,
        # then has nginx re-read its settings ONCE for all the new names.
        vs=/etc/nginx/conf.d/video-servers.conf
        kept=/var/lib/video-servers/learned.conf
        mkdir -p /var/lib/video-servers; touch "$$kept"
        sed -i -E 's/ keepalive_timeout [0-9]+s; }$$/ keepalive_timeout 20s; }/' "$$kept"   # (an idle time from another version of this file)
        # only lines of exactly the form this script writes are taken back in
        grep -E '^upstream [A-Za-z0-9.-]+ \{ zone vs[0-9]+_[0-9]+ 64k; server [A-Za-z0-9.-]+:443 resolve max_fails=0; keepalive 8; keepalive_timeout 20s; \}$$' "$$kept" | head -n 200 > "$$vs" || true
        nginx -t -q 2>/dev/null || : > "$$vs"                   # the kept list is unusable: start without it
        (
          seen=/var/run/video-servers.seen
          : > "$$seen"
          while sleep 5; do
            [ -s "$$seen" ] || continue
            new=$$(sort -u "$$seen"); : > "$$seen"
            fresh=0
            for h in $$new; do grep -qF "upstream $$h {" "$$vs" || fresh=1; done
            [ "$$fresh" = 1 ] || continue
            sleep 20                                            # let the channel changes finish
            new=$$(printf '%s\n%s\n' "$$new" "$$(cat "$$seen")" | sort -u); : > "$$seen"
            [ "$$(wc -l < "$$vs")" -gt 200 ] && : > "$$vs"        # far more than any provider uses: start the list over
            cp "$$vs" "$$vs.before"; added=0
            for h in $$new; do
              case "$$h" in ""|*[!A-Za-z0-9.-]*|.*|-*) continue;; esac  # a plain server name and nothing else
              case "$$h" in *[A-Za-z]*) ;; *) continue;; esac           # not a bare number address
              grep -qF "upstream $$h {" "$$vs" && continue
              n=$$(( $$(wc -l < "$$vs") + 1 ))
              printf 'upstream %s { zone vs%s_%s 64k; server %s:443 resolve max_fails=0; keepalive 8; keepalive_timeout 20s; }\n' "$$h" "$$n" "$$(date +%s)" "$$h" >> "$$vs"
              added=1
            done
            [ "$$added" = 1 ] || continue
            if nginx -t -q 2>/dev/null; then cp "$$vs" "$$kept"; nginx -s reload 2>/dev/null; else cp "$$vs.before" "$$vs"; fi
          done
        ) &
        # nginx itself needs none of the settings (they are in the file written
        # above), so it starts without them.
        # One nginx worker. The image default is one per CPU thread (20 on a
        # 20-thread machine), and each worker keeps its own separate set of open
        # connections to the video servers, so a stream bounced between workers
        # kept opening new ones. A home proxy carries a handful of streams; one
        # worker handles them all and reuses the same connections for all of them.
        sed -i "s/^worker_processes .*/worker_processes 1;/" /etc/nginx/nginx.conf
        exec env -i PATH="$$PATH" TZ="$${TZ:-UTC}" /docker-entrypoint.sh nginx -g 'daemon off;'

    # Healthy = nginx is still inside the tunnel container's network AND the
    # settings were usable (see the status page) AND it answers. If the
    # tunnel container was restarted underneath nginx, nginx is
    # left in a dead network with no address of its own; then the check
    # waits a moment for the tunnel container to be back and stops nginx, so
    # Docker starts it again inside the tunnel's new network.
    healthcheck:
      test: ["CMD-SHELL", "ip -o addr show scope global | grep -q . || { sleep 12; kill 1; exit 1; }; [ ! -s /var/run/proxy-problem ] || exit 1; wget -q -T 5 -O /dev/null http://127.0.0.1:8195/healthz || exit 1"]
      interval: 30s
      timeout: 20s
      start_period: 20s
      retries: 3

  # ---------------------------------------------------------------------------
  # The relay. Plain Python (the text under "relay" at the bottom of this file)
  # in the stock Python image, with nothing installed. It shares the tunnel's
  # network like nginx does, so it has no other way out. nginx hands it three
  # kinds of request: live MPEG-TS (part 6 of nginx's notes), HLS playlists
  # (part 3b) and the status page. The top of the script says what it does
  # with each. If it is not running, TV still plays: nginx passes live
  # MPEG-TS to the provider itself, each reader on a connection of its own,
  # and rewrites playlists itself.
  # ---------------------------------------------------------------------------
  relay:
    image: python:3-alpine
    container_name: iptv-relay
    restart: unless-stopped
    network_mode: "service:vpn"
    user: "65534:65534"                 # needs no privileges
    depends_on:
      vpn:
        condition: service_healthy
        restart: true
    mem_limit: 512m                     # a ceiling: it uses about 30 MB at rest and some tens of MB for each channel being watched
    env_file:
      - path: .env
        required: false
      - path: stack.env
        required: false
    configs:
      - source: relay
        target: /relay.py
    # Of everything in .env, the relay is given only the addresses of the
    # providers that are set up (checked the way nginx checks them). No login,
    # no password and no VPN key is kept in its environment.
    command:
      - sh
      - -c
      - |
        list=""
        for n in $$(env | sed -n 's/^\([A-Z][A-Z0-9_]*\)_URL=.*/\1/p'); do
          case "$$n" in PROXY|PLAYER|VPN|OPENVPN|WIREGUARD|SERVER|FIREWALL|DNS) continue;; esac
          case "$$(printenv "$${n}_LIVE_TV")" in hls|ts) ;; *) continue;; esac
          printenv "$${n}_URL" | grep -Eqx 'https?://[A-Za-z0-9._-]+(:[0-9]{1,5})?/?' || continue
          printenv "$${n}_PORT" | grep -Eqx '[1-9][0-9]{0,4}' || continue
          [ -n "$$(printenv "$${n}_PORT")" ] && [ -n "$$(printenv "$${n}_USER")" ] && [ -n "$$(printenv "$${n}_PASSWORD")" ] || continue
          list="$$list $$(printenv "$${n}_URL")"
        done
        exec env -i PATH="$$PATH" RELAY_PROVIDERS="$$list" python -u /relay.py
    # Healthy = still inside the tunnel container's network AND answering. As
    # with nginx: if the tunnel was restarted underneath it, it stops so that
    # Docker starts it again inside the tunnel's new network.
    healthcheck:
      test: ["CMD-SHELL", "ip -o addr show scope global | grep -q . || { sleep 12; kill 1; exit 1; }; wget -q -T 5 -O /dev/null http://127.0.0.1:8197/healthz || exit 1"]
      interval: 30s
      timeout: 20s
      start_period: 20s
      retries: 3

volumes:
  hls-memory:
  video-servers:

configs:
  proxy:
    content: |
      # Names are looked up through the tunnel container's own DNS.
      resolver 127.0.0.1 ipv6=off valid=60s;
      resolver_timeout 5s;

      # ---- Logging. Xtream addresses contain the login, so the address is never logged.
      map $$uri $$kind {
        default                              api;
        ~^/__cdu/.*\.m3u8$$                   cdn-playlist;
        ~^/__cdu/                            cdn-media;
        ~^/__cdn/.*\.m3u8$$                   cdn-playlist;
        ~^/__cdn/                            cdn-media;
        ~^/__ch/.*\.m3u8$$                    hls;
        ~^/__ch/                             hls-media;
        ~\.m3u8$$                             hls-open;
        ~^/(live|movie|series|timeshift)/    media;
        /xmltv.php                           guide;
      }
      log_format masked '$$remote_addr [$$time_local] $$server_port $$status $$body_bytes_sent $$request_time done=$$request_completion up=$$upstream_status up_bytes=$$upstream_bytes_received up_time=$$upstream_response_time m=$$request_method rng="$$http_range" ua="$$http_user_agent" k=$$kind s=$$sess uh=$$proxy_host uct=$$upstream_connect_time';
      # ---- Video servers. Every request that was answered by a server on the
      # ---- usual secure port, reached by name, has that name noted in a small
      # ---- file. The loop in "entrypoint" above reads it every few seconds and
      # ---- gives each new server a line of its own, so nginx keeps connections
      # ---- to it open and reuses them. Nothing here names a server.
      map $$upstream_status $$vs_ok {
        default        0;
        ~(^|\s)20[06]$$  1;
      }
      # ---- Only video counts: a playlist, a video piece, a stream or a film.
      # ---- Servers that only hand out pictures (logos, posters) are not noted.
      map "$$vs_ok:$$proxy_port:$$proxy_host $$upstream_http_content_type" $$vs_note {
        default                        0;
        "~*^1:443:[A-Za-z0-9.-]+ (video/|audio/|application/(vnd\.apple\.mpegurl|x-mpegurl|mpegurl|octet-stream))"  1;
      }
      log_format vs '$$proxy_host';
      error_log /dev/stderr crit;                # upstream error lines would print the address, login included

      # ---- Plain reverse-proxy settings, the same for everything.
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Accept-Encoding "";      # plain text, so HLS playlists can be read (see sub_filter)
      proxy_set_header X-Hls-Provider "";       # internal notes of the session memory; never accepted from a player
      proxy_set_header X-Hls-Renew "";
      proxy_set_header X-Relay-Provider "";     # internal notes for the relay; never accepted from a player
      proxy_set_header X-Relay-Uri "";
      proxy_set_header X-Relay-Orig "";
      proxy_set_header X-Relay-Port "";
      proxy_set_header X-Relay-User "";
      proxy_set_header X-Relay-Pass "";
      proxy_ssl_server_name on;
      # Certificates are not checked (nginx's default). Many players do not check
      # them either, and some providers' servers have expired or self-made ones;
      # checking here would refuse servers that play fine without the proxy.
      proxy_buffering off;                      # pass bytes on as they arrive
      proxy_request_buffering off;
      proxy_max_temp_file_size 0;
      proxy_buffer_size 64k;
      proxy_buffers 8 64k;
      chunked_transfer_encoding off;            # end streams by closing, like the provider does
      proxy_connect_timeout 10s;
      proxy_read_timeout 60s;                   # longest gap between bytes, not stream length
      send_timeout 60s;

      # ---- 1. Redirects (MPEG-TS, movies, series, catch-up, the guide). A provider
      # ---- answers "go to that other server". Rewrite the address so the player
      # ---- comes back to this proxy for it: /__cdn/<scheme>/<host>/...
      absolute_redirect off;                    # hand the player "/__cdn/..." as is, never "http://this-host/__cdn/..."
      proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
      proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$pscheme/$$1$$2;   # "//server/path": same http or https as the provider

      # ---- 2. Where a /__cdn/ request really goes, and what it may not reach.
      map $$request_uri $$cdn_target {
        default "";
        ~*^/__cdn/(?<cs>https?)/(?<ch>[^/?]+)(?<cr>/.*)$$  $$cs://$$ch$$cr;
        ~*^/__cdn/(?<cs>https?)/(?<ch>[^/?]+)$$            $$cs://$$ch/;
      }
      map $$request_uri $$cdn_prefix {
        default "";
        ~*^(?<cp>/__cdn/https?/[^/?]+)  $$cp;
      }
      map $$request_uri $$cdn_scheme {
        default http;
        ~*^/__cdn/https/  https;
      }
      map $$request_uri $$cdn_forbidden {         # nothing on this machine or this network
        default 0;
        "~*^/__cdn/https?/(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|([0-9]+\.)*0[0-9]+(\.[0-9]+)*([:/?]|$$)|\[|[^/]*[@%])"  1;
      }

      # ---- 2b. /__cdu/: the same as /__cdn/, for one kind of address only: a
      # ---- "../" line from a live channel's playlist (see $$cdn_up in part 3).
      # ---- Such an address is /__cdu/<scheme>/<server>, then 32 spare folders
      # ---- named "__up", then one named "__top", then the playlist's real folder
      # ---- and the "../" line itself. The player does the climbing. Climbing
      # ---- inside the real folders leaves the spares alone (first rule below).
      # ---- Climbing higher than the server's top folder uses up "__top" and then
      # ---- spares, never the server's name, and what is left is counted from
      # ---- the top folder (second rule), which is where a player talking to the
      # ---- server directly ends up too. /__cdn/ itself is not changed in any way.
      map $$request_uri $$cdu_target {
        default "";
        "~*^/__cdu/(?<ua>https?)/(?<ub>[^/?]+)(/__up){32}/__top(?<uc>/.*)$$"  $$ua://$$ub$$uc;
        "~*^/__cdu/(?<ua>https?)/(?<ub>[^/?]+)(/__up)*(?<uc>/.*)$$"           $$ua://$$ub$$uc;
      }
      map $$request_uri $$cdu_server {             # the server alone
        default "";
        "~*^/__cdu/(?<ue>https?)/(?<uf>[^/?]+)"  $$ue://$$uf;
      }
      map $$request_uri $$cdu_prefix {             # the same server's ordinary /__cdn/ address
        default "";
        "~*^/__cdu/(?<ug>https?)/(?<ui>[^/?]+)"  /__cdn/$$ug/$$ui;
      }
      map $$request_uri $$cdu_scheme {
        default http;
        ~*^/__cdu/https/  https;
      }
      map $$request_uri $$cdu_forbidden {         # nothing on this machine or this network (the same list as for /__cdn/)
        default 0;
        "~*^/__cdu/https?/(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|([0-9]+\.)*0[0-9]+(\.[0-9]+)*([:/?]|$$)|\[|[^/]*[@%])"  1;
      }
      # A few players send ".." as written instead of working it out. Normally
      # that is passed on untouched and the video server works it out, as it would
      # without the proxy. Only when such an address climbs higher than the
      # server's top folder (the spares are no longer all there once nginx has
      # worked the path out) is nginx's worked-out path used instead.
      map $$request_uri $$cdu_dots {
        default 0;
        "~*^/__cdu/[^?]*/\.\.?(/|\?|$$)"  1;
      }
      map $$uri $$cdu_state {
        default lost;                              # climbed past the spares and the server's name: nothing left to go by
        "~*^/__cdu/https?/[^/]+(/__up){32}/__top(/|$$)"  intact;
        "~*^/__cdu/https?/[^/]+(/__up)*/"               over;
      }
      map "$$cdu_dots:$$cdu_state" $$cdu_fix {
        default  ok;
        "1:over" over;
        "1:lost" lost;
      }

      # ---- 3. HLS playlists name their video pieces without a server (or with the
      # ---- other server's full address). Point those lines back at this proxy too.
      # http or https of the server the playlist in hand came from (for "//server/path")
      map "$$cdn|$$pscheme" $$cdn_rel {
        default            /__cdn/https;
        ~^/__cdn/http/     /__cdn/http;
        ~^\|http$$         /__cdn/http;
      }
      sub_filter_types application/vnd.apple.mpegurl application/x-mpegurl application/mpegurl audio/mpegurl audio/x-mpegurl;
      sub_filter_once off;
      # An address written as "//server/path" means "same http or https as this
      # playlist, on that other server". These two rules must stay ABOVE the
      # "/..." rules below: nginx uses the first rule that fits.
      sub_filter "\n//"           "\n$$sf_rel/";
      sub_filter 'URI="//'        'URI="$$sf_rel/';
      sub_filter "\n/"            "\n$$sf_cdn/";
      sub_filter 'URI="/'         'URI="$$sf_cdn/';
      sub_filter "\nhttps://"     "\n$$sf_https";
      sub_filter "\nhttp://"      "\n$$sf_http";
      sub_filter 'URI="https://'  'URI="$$sf_https';
      sub_filter 'URI="http://'   'URI="$$sf_http';
      # An address written as "../folder/file" means "up one folder from this
      # playlist". A live channel's playlist is handed to the player in a made-up
      # folder (/__ch/..., see part 4), where "up one folder" leads nowhere. For
      # those playlists $$cdn_up is the playlist's real folder on its own server,
      # reached through /__cdu/ (part 2b), followed by "../", so the player does
      # the climbing on the real address ("../../x" works the same way).
      # Everywhere else $$cdn_up is just "../" and these two rules change nothing:
      # a playlist fetched through /__cdn/ or /__cdu/ sits at its real address
      # already, and its "../" lines are passed on exactly as they are.
      # Not covered, because nothing is left to go by: a line that climbs more
      # than 32 folders higher than the server's top folder. Not rewritten, and
      # so no better than before: "./../x", "folder/../../x" and a bare "..".
      sub_filter "\n../"          "\n$$sf_up";
      sub_filter 'URI="../'       'URI="$$sf_up';
      # ---- Who does this rewriting. A player's request for a playlist is handed to
      # ---- the relay (part 3b in provider-locations), which reads the playlist
      # ---- line by line, the way a player does, and so copes with forms these
      # ---- text rules would miss. The relay asks nginx for the playlist from the
      # ---- address 127.0.0.2. For that one caller the rules above put back
      # ---- exactly what they found ($$sf_... below), so the playlist arrives as
      # ---- the provider wrote it, and three notes say where its addresses lead.
      # ---- For every other caller the rules work as described: that is what a
      # ---- player gets if the relay is not running (asked from 127.0.0.3).
      map $$remote_addr $$pl_raw   { default 0; 127.0.0.2 1; }
      map $$remote_addr $$pl_inner { default 0; 127.0.0.2 1; 127.0.0.3 1; }
      map $$pl_raw $$sf_rel   { default $$cdn_rel;       1 "/"; }
      map $$pl_raw $$sf_cdn   { default $$cdn;           1 ""; }
      map $$pl_raw $$sf_up    { default $$cdn_up;        1 "../"; }
      map $$pl_raw $$sf_https { default "/__cdn/https/"; 1 "https://"; }
      map $$pl_raw $$sf_http  { default "/__cdn/http/";  1 "http://"; }
      map $$pl_raw $$pl_note_cdn { default ""; 1 $$cdn; }
      map $$pl_raw $$pl_note_rel { default ""; 1 $$cdn_rel; }
      map $$pl_raw $$pl_note_up  { default ""; 1 $$cdn_up; }
      add_header X-Pl-Cdn $$pl_note_cdn;
      add_header X-Pl-Rel $$pl_note_rel;
      add_header X-Pl-Up  $$pl_note_up;
      proxy_hide_header X-Pl-Cdn;               # only nginx writes these notes: a server that sends its own is not believed
      proxy_hide_header X-Pl-Rel;
      proxy_hide_header X-Pl-Up;
      # which requests are playlists a player is asking for
      map "$$request_method:$$pl_inner:$$uri" $$pl_via {
        default 0;
        "~^GET:0:/__ch/.*\.m3u8$$"          1;
        "~^GET:0:/__c(dn|du)/.*\.m3u8$$"    1;
      }

      # ---- 4. Live HLS: one provider session per channel.
      # ---- Many providers start a NEW session every time a channel's address is
      # ---- asked, and a new session ends the one before it. A player refreshes
      # ---- that address every few seconds, and several players or a recording may
      # ---- be on the same channel. So the proxy remembers where the provider sent
      # ---- a channel and keeps everyone on that session: the pieces keep the same
      # ---- addresses from one refresh to the next, and nobody ends anyone else's
      # ---- stream. When the remembered session is gone, it is replaced once.
      proxy_cache_path /var/cache/nginx/hls levels=1 keys_zone=hls:1m max_size=16m inactive=1m use_temp_path=off;

      server {                                  # the memory itself; reachable only from this container
        listen 127.0.0.1:8195;
        access_log off;
        location = /healthz { return 200; }     # the container's health check
        location / {
          set $$cdn "";
          set $$cdn_up "../";
          set $$sess "-";                       # (these keep the config valid when no provider is set up)
          set $$pscheme http;
          proxy_redirect off;
          proxy_buffering on;                   # needed to remember an answer; this one is only a redirect
          proxy_cache hls;
          proxy_cache_key "$$http_x_hls_provider$$uri";   # the channel address; a query string never names another channel
          proxy_cache_valid 301 302 303 307 308 365d;  # only the "it is over there" answer is remembered, for as long as the channel is in use
          proxy_cache_lock on;                  # opens arriving together wait for the first one
          proxy_cache_lock_timeout 20s;         # ...for as long as a slow provider may take to answer; with a shorter
          proxy_cache_lock_age 20s;             # wait, the later opens went to the provider too and ended the first session
          proxy_cache_bypass $$http_x_hls_renew; # asked to replace a session that is gone
          proxy_ignore_headers Cache-Control Expires Set-Cookie Vary X-Accel-Expires;
          add_header X-Hls-Sess $$upstream_cache_status always;
          proxy_pass $$http_x_hls_provider;
        }
      }

      # ---- PROVIDERS: one server block per provider is built at start from
      # ---- .env (see "entrypoint" above) and shares provider-locations below.

  # Shared by every provider block (one per provider in .env).
  provider-locations:
    content: |
      access_log /dev/stdout masked;
      access_log /var/run/video-servers.seen vs if=$$vs_note;   # see "Video servers" above
      set $$sess   "-";
      set $$cdn    "";
      set $$cdn_up "../";                     # "../" in a playlist stays "../" unless a channel-folder playlist says otherwise (see part 3)
      set $$chmode "";
      set $$pbase  "";
      set $$easy_user "${PLAYER_USER}";       # for the status page, which asks each provider for its account details
      set $$easy_pass "${PLAYER_PASSWORD}";

      # ---- The address the player asked for. Normally that is the request
      # ---- itself. A live HLS channel is handed its own folder, /__ch/<the
      # ---- channel's address>/ (see part 4), so there the channel's address is
      # ---- taken back out of the request.
      set $$orig $$request_uri;
      if ($$request_uri ~ "^(?<rp>[^?]*)(?<rq>\?.*)?$$") { set $$chmode ""; }
      if ($$request_uri ~ "^/__ch(?<co>/.*?\.m3u8)/(?<cn>[^?]*)(?<cq>\?.*)?$$") { set $$orig $$co; set $$chmode piece; }
      if ($$request_uri ~ "^/__ch(?<cop>/.*?\.m3u8)/__index\.m3u8(?<cqp>\?.*)?$$") { set $$orig "$$cop$$cqp"; set $$chmode playlist; }

      # ---- 5. One easy login for everything behind this proxy. A request that
      # ---- logs in as PLAYER_USER is sent on with the provider account of its
      # ---- port; any other login is passed on untouched (and refused by the provider).
      # ---- $$up_uri is the address sent to the provider: the player's own address,
      # ---- byte for byte, with only the login swapped. $$acct_user / $$acct_pass are
      # ---- already percent-encoded (see "entrypoint"), so any character is safe.
      set $$up_uri $$orig;
      # (named pieces, not $$1 $$2: nginx re-encodes numbered ones when the address contains %XX)
      if ($$orig ~ "^/+(?<pa>live|movie|series|timeshift)/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+(?<pb>.*)$$") { set $$up_uri "/$$pa/$$acct_user/$$acct_pass/$$pb"; }
      if ($$orig ~ "^/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+(?<pc>[^/?]+(\?.*)?)$$") { set $$up_uri "/$$acct_user/$$acct_pass/$$pc"; }
      if ($$up_uri ~ "^(?<qa>[^?]*\?(.*&)?)username=${PLAYER_USER}(?<qb>(&.*)?)$$") { set $$up_uri "$${qa}username=$$acct_user$$qb"; }
      if ($$up_uri ~ "^(?<qc>[^?]*\?(.*&)?)password=${PLAYER_PASSWORD}(?<qd>(&.*)?)$$") { set $$up_uri "$${qc}password=$$acct_pass$$qd"; }

      # ---- 3b. A playlist a player asks for goes to the relay to have its addresses
      # ---- pointed back at this proxy (see "Who does this rewriting" above). If the
      # ---- relay is not running, nginx does it itself, as it always has.
      recursive_error_pages on;                 # so that @pl_relay can step on to @pl_plain
      error_page 418 = @pl_relay;
      if ($$pl_via) { return 418; }
      location @pl_relay {
        sub_filter_types application/x-nothing;   # the relay's answer is finished: the rules above must not run over it again,
        chunked_transfer_encoding on;             # and the player's connection stays open for the next refresh
        proxy_intercept_errors off;
        proxy_redirect off;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_set_header X-Relay-Orig $$request_uri;
        proxy_set_header X-Relay-Port $$server_port;
        proxy_connect_timeout 2s;
        proxy_read_timeout 35s;                   # a little longer than the relay itself waits for a playlist; then nginx does it alone
        proxy_set_header X-Relay-Provider "";
        proxy_set_header X-Relay-Uri "";
        proxy_set_header X-Relay-User "";
        proxy_set_header X-Relay-Pass "";
        error_page 502 504 = @pl_plain;
        set $$pl_to http://127.0.0.1:8197/pl;   # (written this way because nginx wants it so in a named location)
        proxy_pass $$pl_to;
      }
      location @pl_plain {
        sub_filter_types application/x-nothing;   # rewritten once already, by the request this one hands on to
        chunked_transfer_encoding on;
        proxy_intercept_errors off;
        proxy_redirect off;
        proxy_set_header Connection "";
        proxy_set_header Host $$http_host;
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_bind 127.0.0.3;
        proxy_pass http://127.0.0.1:$$server_port$$request_uri;
      }
      location = /healthz { access_log off; return 200; }
      location / { proxy_pass $$provider$$up_uri; }

      # The guide and the API are text, often large. Let the provider compress
      # them if the app asks (as it would directly), and keep the framing that
      # lets the app notice a download that was cut short.
      location = /xmltv.php {
        proxy_read_timeout 300s;
        chunked_transfer_encoding on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding $$http_accept_encoding;
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_pass $$provider$$up_uri;
      }
      # The provider's lists (channels, movies, series). They give logos and
      # posters as full addresses, usually on other sites. Those addresses are
      # pointed back at this proxy, so the player fetches the pictures through
      # the VPN too. For that the list has to arrive as plain text; it is
      # compressed again for the app if the app asks.
      location = /player_api.php {
        proxy_read_timeout 300s;
        chunked_transfer_encoding on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        gzip on;
        gzip_types application/json text/plain;
        gzip_proxied any;
        gzip_min_length 1024;
        sub_filter_types *;
        sub_filter_once off;
        sub_filter '"https:\/\/' '"http://$$http_host/__cdn/https/';
        sub_filter '"http:\/\/'  '"http://$$http_host/__cdn/http/';
        sub_filter '"https://'   '"http://$$http_host/__cdn/https/';
        sub_filter '"http://'    '"http://$$http_host/__cdn/http/';
        proxy_pass $$provider$$up_uri;
      }

      # ---- The status page: what is set up, what is wrong, where the VPN comes out.
      # ---- Drawn by the relay; the few lines that hand it over are written at start
      # ---- (see "entrypoint" above).
      include /etc/nginx/status.inc;

      # ---- 6. Live MPEG-TS: one provider connection per channel here as well.
      # ---- Part 4 keeps every HLS reader of a channel on one provider session. A
      # ---- request for a live channel as MPEG-TS (".ts", which many recorders
      # ---- use) would open another one, and with a provider that allows one per
      # ---- channel the readers then end each other over and over: the picture
      # ---- freezes every few seconds and a recording is cut to pieces.
      # ---- Live ".ts" requests are handed to the relay (the small Python program
      # ---- beside nginx; its text is at the bottom of this file). What it does
      # ---- with them is set for each provider by its NAME_LIVE_TV line in .env.
      # ---- There is no default, because the two choices suit different homes:
      # ----   ..._LIVE_TV=hls  Your players use HLS. A ".ts" reader is given the
      # ----                channel's HLS video, joined end to end: the relay
      # ----                fetches the same playlist and pieces the players get,
      # ----                from this proxy, so everyone is on one provider
      # ----                session. Watching and recording a channel at once
      # ----                works whichever started first. That stream is the HLS
      # ----                one: a few seconds further behind live than the
      # ----                provider's MPEG-TS.
      # ----   ..._LIVE_TV=ts   Your players use MPEG-TS. The first reader of a channel
      # ----                makes the relay open it at the provider, and every
      # ----                reader of that channel is given the provider's own
      # ----                bytes. The provider sees one connection however many
      # ----                readers there are, one leaving does not disturb the
      # ----                rest, and the connection is dropped the moment the
      # ----                last one leaves. The one who opens a channel gets the
      # ----                stream from its first byte; one who joins is handed the
      # ----                newest 10 seconds of video at once (worked out from how
      # ----                fast that channel's video comes), on a whole packet, so
      # ----                it has something in hand. Opening a channel as
      # ----                HLS while it is being read as MPEG-TS still ends one
      # ----                of the two: that is the provider's rule, and turning
      # ----                MPEG-TS into HLS is beyond this proxy.
      # ---- One more thing the relay does for MPEG-TS: if the provider ends a
      # ---- stream while people are watching, the relay asks for it again at once
      # ---- and carries on, so the players do not each have to notice and ask
      # ---- again by themselves. (Only for a stream that had run 30 seconds.)
      # ---- For nginx these are ordinary proxied requests: it passes the bytes on
      # ---- and, when the player hangs up, closes its connection to the relay,
      # ---- which is how the relay knows a reader has gone.
      location ~ "^/+live/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+[^/]+\.ts$$" {
        proxy_intercept_errors off;
        proxy_read_timeout 100s;
        proxy_set_header Connection "";
        proxy_set_header Range "";                       # a live stream has no ranges
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_set_header X-Relay-Provider $$provider;    # which provider (the relay accepts only those in .env)
        proxy_set_header X-Relay-Uri $$up_uri;           # the channel's address there, with the provider's login
        proxy_set_header X-Relay-Orig $$orig;            # the address as the player sent it
        proxy_set_header X-Relay-Port $$server_port;
        proxy_set_header X-Relay-User "";
        proxy_set_header X-Relay-Pass "";
        proxy_connect_timeout 2s;
        error_page 502 = @ts_plain;                      # the relay is not running: the provider directly
        proxy_pass http://127.0.0.1:8197/$$live_tv;
      }
      # the short forms of a live address (no "/live/", with or without ".ts")
      location ~ "^/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+[0-9]+(\.ts)?$$" {
        proxy_intercept_errors off;
        proxy_read_timeout 100s;
        proxy_set_header Connection "";
        proxy_set_header Range "";                       # a live stream has no ranges
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_set_header X-Relay-Provider $$provider;    # which provider (the relay accepts only those in .env)
        proxy_set_header X-Relay-Uri $$up_uri;           # the channel's address there, with the provider's login
        proxy_set_header X-Relay-Orig $$orig;            # the address as the player sent it
        proxy_set_header X-Relay-Port $$server_port;
        proxy_set_header X-Relay-User "";
        proxy_set_header X-Relay-Pass "";
        proxy_connect_timeout 2s;
        error_page 502 = @ts_plain;                      # the relay is not running: the provider directly
        proxy_pass http://127.0.0.1:8197/$$live_tv;
      }
      location @ts_plain { proxy_pass $$provider$$up_uri; }

      # A provider's ready-made M3U playlist lists every stream at the
      # provider's own address with the real login, so a player using it would
      # skip the VPN. Refused on purpose: use the Xtream login instead.
      location = /get.php { return 403; }

      # (Order matters below: nginx tries these patterns from top to bottom, so the
      # two for a channel's own folder come before the general ".m3u8" one.)

      # ---- Live HLS, the channel's playlist (the player arrives here from "the start", further down). Fetch the remembered
      # ---- session's playlist. The player always asks the same address and always
      # ---- gets the same session.
      location ~ ^/__ch/.+\.m3u8/__index\.m3u8$$ {
        chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
        if ($$chmode != playlist) { return 404; }
        sub_filter_types *;
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls {
        chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
        sub_filter_types *;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_read_timeout 25s;               # long enough for a channel that is slow to start, short enough to replace one that hangs
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }        # "//server/path"
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }     # "/some/path" with no server name
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|([0-9]+\.)*0[0-9]+(\.[0-9]+)*([:/?]|$$)|\[|[^/]*[@%])") { return 403; }
        if ($$next ~* "^(https?)://([^/]+)") { set $$cdn "/__cdn/$$1/$$2"; }
        if ($$next ~* "^(?<us>https?)://(?<uh>[^/?]+)(?<ud>/([^?]*/)?)") { set $$cdn_up "/__cdu/$$us/$$uh/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__top$$ud../"; }   # the playlist's real folder, for "../" lines
        error_page 400 401 403 404 410 429 500 502 503 504 509 520 521 522 523 524 = @hls_renew;   # the remembered session is gone
        proxy_pass $$next;
      }
      location @hls_renew {                   # open the channel again, once, and remember the new session
        chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew 1;
        error_page 301 302 303 307 308 = @hls_renewed;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_renewed {                 # whatever this one answers goes to the player
        chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
        sub_filter_types *;
        proxy_read_timeout 25s;
        set $$cdn_up "../";                   # not the folder of the session that was just replaced
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|([0-9]+\.)*0[0-9]+(\.[0-9]+)*([:/?]|$$)|\[|[^/]*[@%])") { return 403; }
        if ($$next ~* "^(https?)://([^/]+)") { set $$cdn "/__cdn/$$1/$$2"; }
        if ($$next ~* "^(?<us>https?)://(?<uh>[^/?]+)(?<ud>/([^?]*/)?)") { set $$cdn_up "/__cdu/$$us/$$uh/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__top$$ud../"; }   # the playlist's real folder, for "../" lines
        proxy_pass $$next;
      }

      # ---- Live HLS, the pieces: anything else asked inside a channel's folder is a
      # ---- piece (or a further playlist) that the channel's playlist named with
      # ---- no server and no folder. It lives next to that playlist, so fetch it
      # ---- from the folder the channel's session is in.
      location ~ ^/__ch/ {
        chunked_transfer_encoding on;         # a further playlist in the channel's folder keeps the connection open too (pieces carry their length and are not affected)
        if ($$chmode != piece) { return 404; }
        limit_except GET HEAD { deny all; }
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls_piece;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_piece {
        chunked_transfer_encoding on;
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|([0-9]+\.)*0[0-9]+(\.[0-9]+)*([:/?]|$$)|\[|[^/]*[@%])") { return 403; }
        set $$pdir "/";
        if ($$next ~* "^https?://[^/?]+(?<nd>/[^?]*/)") { set $$pdir $$nd; }
        if ($$next ~* "^(?<ns>https?)://(?<nh>[^/?]+)") { set $$cdn "/__cdn/$$ns/$$nh"; set $$pbase "$$ns://$$nh"; }
        if ($$pbase = "") { return 502; }
        set $$cn_dir "";                       # a further playlist in the channel's folder: its own real folder, for "../" lines
        if ($$cn ~ "^(?<cnf>.*/)[^/]*$$") { set $$cn_dir $$cnf; }
        set $$cdn_up "/__cdu/$$ns/$$nh/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__up/__top$$pdir$${cn_dir}../";
        proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
        proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$ns/$$1$$2;
        proxy_redirect ~^(/[^/].*)$$ $$cdn$$1;
        proxy_pass $$pbase$$pdir$$cn$$cq;
      }

      # ---- Live HLS, the start: the channel is opened. Ask the memory where this
      # ---- channel's session is. If the provider sends the channel to another
      # ---- server (most do), give the channel a folder of its own on this proxy
      # ---- and send the player there. A playlist may name its pieces with no
      # ---- folder at all ("chunk_123.ts"); the player then asks for them inside
      # ---- that same folder, and "the pieces" above knows which channel they belong to.
      # ---- If the provider answers with the playlist itself, it is passed on as is.
      location ~ \.m3u8$$ {
        chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
        sub_filter_types *;                   # a playlist is rewritten whatever type the provider labels it
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls_open;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_open {
        set $$sess $$upstream_http_x_hls_sess;
        return 302 /__ch$$rp/__index.m3u8$$rq;
      }

      # Everything a provider pointed at: video pieces, MPEG-TS, movies, the guide.
      location ^~ /__cdn/ {
        set $$cdn $$cdn_prefix;
        set $$cdn_s $$cdn_scheme;
        if ($$cdn_forbidden) { return 403; }
        if ($$cdn_target = "") { return 404; }
        limit_except GET HEAD { deny all; }   # players only ever fetch
        proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
        proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$cdn_s/$$1$$2;
        proxy_redirect ~^(/[^/].*)$$ $$cdn$$1;   # "go to /some/path" on the same server
        proxy_pass $$cdn_target;

        # A playlist fetched from the other server: rewritten whatever type it is labelled.
        location ~ \.m3u8$$ {
          chunked_transfer_encoding on;         # a playlist keeps the connection open, as the provider's own server does
          sub_filter_types *;
          set $$cdn $$cdn_prefix;
          set $$cdn_s $$cdn_scheme;
          if ($$cdn_forbidden) { return 403; }
          if ($$cdn_target = "") { return 404; }
          limit_except GET HEAD { deny all; }
          proxy_pass $$cdn_target;
        }
      }

      # A "../" line from a live channel's playlist (see part 2b). The same as
      # /__cdn/ above in every other way.
      location ^~ /__cdu/ {
        set $$cdn $$cdu_prefix;
        set $$cdn_s $$cdu_scheme;
        set $$cdu_to $$cdu_target;
        if ($$cdu_forbidden) { return 403; }
        if ($$cdu_target = "") { return 404; }
        limit_except GET HEAD { deny all; }
        proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
        proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$cdn_s/$$1$$2;
        proxy_redirect ~^(/[^/].*)$$ $$cdn$$1;
        if ($$cdu_fix = lost) { return 404; }
        if ($$cdu_fix = over) { set $$cdu_to $$cdu_server; rewrite "(?i)^/__cdu/https?/[^/]+(/__up)*(?<dn>/.*)$$" $$dn break; }   # ".." as written, climbing too high: nginx's worked-out path, counted from the top folder
        proxy_pass $$cdu_to;

        location ~ \.m3u8$$ {
          chunked_transfer_encoding on;
          sub_filter_types *;
          set $$cdn $$cdu_prefix;
          set $$cdn_s $$cdu_scheme;
          set $$cdu_to $$cdu_target;
          if ($$cdu_forbidden) { return 403; }
          if ($$cdu_target = "") { return 404; }
          limit_except GET HEAD { deny all; }
          if ($$cdu_fix = lost) { return 404; }
          if ($$cdu_fix = over) { set $$cdu_to $$cdu_server; rewrite "(?i)^/__cdu/https?/[^/]+(/__up)*(?<dn>/.*)$$" $$dn break; }
          proxy_pass $$cdu_to;
        }
      }

  # The relay (see the "relay" service above). Plain Python, standard library only.
  relay:
    content: |
      # The relay. One small program that does the jobs plain nginx settings cannot:
      #
      #   /ts      Share one provider MPEG-TS stream between every reader of a channel.
      #            The first reader of a channel makes the relay open it at the
      #            provider; everyone else on that channel is given the same bytes. The
      #            provider sees one connection however many readers there are. It is
      #            dropped when the last reader leaves (in a channel's first seconds,
      #            a moment later: see LINGER).
      #   /hls     Give a reader that asks for MPEG-TS (many recorders do) the channel's
      #            HLS video instead, joined end to end, so that it shares the one HLS
      #            session your players are on and does not open a second one.
      #   /pl      Point the addresses inside an HLS playlist back at this proxy, so the
      #            player fetches every piece through the VPN. nginx fetches the
      #            playlist (and keeps doing all the network work); the relay only
      #            reads it line by line and rewrites the addresses.
      #   /status  Draw the status page: what is set up, what is wrong, and where the
      #            VPN comes out.
      #
      # nginx is the only thing that sends it requests (it listens on 127.0.0.1). It
      # connects to the providers named in .env and the servers they redirect to, to
      # nginx in this stack, and, for the status page only, to ipinfo.io to learn
      # where the VPN comes out. It keeps the newest 15 seconds or so of each open channel in memory
      # (never less than 12 MB of it, never more than 64 MB).
      import asyncio, html, http, ipaddress, json, os, re, signal, socket, ssl, sys, time
      from collections import deque
      from urllib.parse import quote, urlsplit, urljoin

      if sys.version_info < (3, 12):   # older asyncio can lose a cancel inside wait_for and leave a provider connection open
          raise SystemExit('RELAY: needs Python 3.12 or newer')

      PORT = 8197
      # A reader that joins a channel already open starts JOIN_SECONDS behind live: it is handed that
      # much of the newest video at once, so it has something in hand when the provider pauses, as a
      # player on a connection of its own has from the provider's opening burst. It is counted in
      # seconds, not megabytes, so it is the same for a small channel and a 4K one: the relay measures
      # how fast each channel's video really comes (over the last minute) and works the amount out
      # from that.
      JOIN_SECONDS = 10
      # What is kept of each open channel, for readers that join: KEEP_SECONDS of it, and never less
      # than RING or more than RING_MAX.
      KEEP_SECONDS = 15
      RING = 12 * 1024 * 1024
      RING_MAX = 64 * 1024 * 1024
      BURST_SECONDS = 2.5           # (the provider's opening burst is measured and noted in the log)
      # A provider that ends a live stream while people are watching is asked for it again, at once, so
      # that every player does not have to notice and ask by itself. Only a stream that had been running
      # for REOPEN_AFTER seconds is asked for again.
      REOPEN_AFTER = 30
      REOPEN_WAIT = 5               # seconds a provider has to answer when asked again (and twice that to send video)
      DROP_GAP = 0.1                # seconds a provider is given to see a hang-up before another channel is asked for
      # A player that is tuning opens a channel, hangs up and asks again within a moment. So a channel is
      # kept for LINGER after its last reader leaves, in its first seconds only; a request for any OTHER
      # channel of that provider drops it at once.
      LINGER = 0.4
      READERS_MAX = 100             # readers the relay serves at once, all channels together
      # A reader that cannot keep up is disconnected, so that what is held for it stays small: when it
      # has been more than BEHIND bytes behind its channel for BEHIND_SECONDS, or FAR_BEHIND at any moment.
      # (This is also about how long a paused player is waited for. A reader that has just joined is
      # allowed what it was handed on top, until it has taken it.)
      BEHIND = 16 * 1024 * 1024
      BEHIND_SECONDS = 15
      FAR_BEHIND = 64 * 1024 * 1024
      CONNECT_WAIT = 10             # seconds for a server to accept a connection
      HEADER_WAIT = 20              # seconds for a server to send each line of its answer's heading
      PLAYLIST_WAIT = 30            # the same for a playlist asked of nginx: longer than nginx's own wait for the provider
      START_WAIT = 60               # seconds a reader waits for a channel's first video
      QUIET_WAIT = 60               # seconds a stream may send nothing before it is given up on
      HEADER_LINES = 100            # an answer's heading may not be longer than this
      # Where a provider's redirect may not lead: this machine, and home and private networks.
      PRIVATE = [ipaddress.ip_network(n) for n in (
          '0.0.0.0/8', '10.0.0.0/8', '100.64.0.0/10', '127.0.0.0/8', '169.254.0.0/16', '172.16.0.0/12',
          '192.168.0.0/16', '::/127', 'fc00::/7', 'fe80::/10')]

      def say(msg):                 # the relay's log lines never hold an address or a login
          print('RELAY: ' + msg, flush=True)

      # The providers this relay may connect to: the addresses nginx was given (see the service's
      # "command" in the compose file, which passes on those and nothing else from .env).
      PROVIDERS = set(u.rstrip('/') for u in os.environ.get('RELAY_PROVIDERS', '').split() if u.lower().startswith(('http://', 'https://')))

      TLS = ssl.create_default_context()
      TLS.check_hostname = False    # certificates are not checked, the same as in nginx (see the notes there)
      TLS.verify_mode = ssl.CERT_NONE
      CHECKED = ssl.create_default_context()   # for the status page's one lookup outside (where the VPN comes out)

      # ---------------------------------------------------------------------------
      # A plain HTTP request, with the answer's body given back piece by piece.
      # ---------------------------------------------------------------------------
      class Heading(dict):
          """An answer's heading: by name (lower case, last one wins) and, in .all, every line as it came."""
          def __init__(self):
              super().__init__()
              self.all = []

      class Refused(Exception):
          """The other side said no (or did not answer properly). Carries what to tell the reader."""
          def __init__(self, status, kind='', text=b''):
              self.status, self.kind, self.text = status, kind, text

      def private(address):
          ip = ipaddress.ip_address(address.split('%')[0])
          if ip.version == 6 and ip.ipv4_mapped:
              ip = ip.ipv4_mapped
          return any(ip in net for net in PRIVATE if net.version == ip.version)

      async def fetch(url, agent, hops=0, guard=True, tls=None, source=None, follow=True, stay=False, wait=HEADER_WAIT):
          """Open url. Returns (status, headers, reader, writer); follows redirects."""
          u = urlsplit(url)
          secure = u.scheme == 'https'
          host, port = u.hostname or '', u.port or (443 if secure else 80)
          if u.scheme not in ('http', 'https') or not host or '@' in u.netloc:
              raise Refused(502)
          # The name is looked up here, and the connection goes to the very address that was checked.
          # (An address written as numbers needs no looking up.)
          try:
              address = str(ipaddress.ip_address(host))
          except ValueError:
              found = await asyncio.wait_for(asyncio.get_running_loop().getaddrinfo(host, port, type=socket.SOCK_STREAM), CONNECT_WAIT)
              address = found[0][4][0]
          if guard and hops and private(address):
              raise Refused(403)    # a redirect may not lead to this machine or to a home network
          r, w = await asyncio.wait_for(asyncio.open_connection(
              address, port, ssl=(tls or TLS) if secure else None, server_hostname=host if secure else None, limit=65536,
              local_addr=(source, 0) if source else None), CONNECT_WAIT)
          try:
              path = (u.path or '/') + ('?' + u.query if u.query else '')
              if isinstance(agent, str):                # the relay's own fetches: just a name. A channel carries the player's own headers.
                  agent = ([('User-Agent', agent)] if agent else []) + [('Accept', '*/*')]
              lines = ['GET %s HTTP/1.1' % path, 'Host: %s' % u.netloc] + ['%s: %s' % nv for nv in agent] + ['Connection: close', '', '']
              w.write('\r\n'.join(lines).encode('latin-1', 'replace'))
              await w.drain()
              line = (await asyncio.wait_for(r.readline(), wait)).decode('latin-1').split()
              status = int(line[1]) if len(line) > 1 and line[1].isdigit() else 502
              headers = Heading()
              for _ in range(HEADER_LINES + 1):
                  h = (await asyncio.wait_for(r.readline(), wait)).decode('latin-1')
                  if h in ('\r\n', '\n', ''):
                      break
                  name, _, value = h.partition(':')
                  headers[name.strip().lower()] = value.strip()
                  headers.all.append((name.strip(), value.strip()))
              else:
                  raise Refused(502)
              if follow and status in (301, 302, 303, 307, 308) and headers.get('location') and hops < 10:
                  shut(w)
                  onward = urljoin(url, headers['location'])
                  if stay and urlsplit(onward).netloc != u.netloc:
                      raise Refused(502)                # (a fetch from nginx in this stack is never sent elsewhere)
                  return await fetch(onward, agent, hops + 1, guard, tls, None, True, stay, wait)
              return status, headers, r, w
          except ValueError:        # a line far too long to be an answer's heading
              shut(w)
              raise Refused(502)
          except BaseException:
              shut(w)
              raise

      def shut(w):
          try:
              w.transport.abort()   # at once: the provider's line must be free before the next channel is asked for
          except Exception:
              pass

      async def body(r, headers, wait=QUIET_WAIT):
          """The pieces of an answer's body, whichever way the server marks its end.
          A body that stops short of its stated length is an error, not an ending."""
          if 'chunked' in headers.get('transfer-encoding', '').lower():
              while True:
                  size = int((await asyncio.wait_for(r.readline(), wait)).split(b';')[0].strip() or b'-1', 16)
                  if size == 0:
                      return
                  if size < 0:
                      raise OSError('not a chunk size')
                  while size:
                      data = await asyncio.wait_for(r.read(min(size, 65536)), wait)
                      if not data:
                          raise OSError('the answer stopped short')
                      size -= len(data)
                      yield data
                  await asyncio.wait_for(r.readline(), wait)
          left = int(headers['content-length']) if headers.get('content-length', '').isdigit() else None
          while left is None or left > 0:
              data = await asyncio.wait_for(r.read(65536 if left is None else min(left, 65536)), wait)
              if not data:
                  if left:
                      raise OSError('the answer stopped short')
                  return
              if left is not None:
                  left -= len(data)
              yield data

      async def get(url, agent, limit=64 * 1024 * 1024):
          """A whole small answer (a playlist, one video piece). Returns (status, bytes)."""
          status, headers, r, w = await fetch(url, agent, guard=False, stay=True)
          try:
              out, n = [], 0
              async for data in body(r, headers, HEADER_WAIT):
                  out.append(data)
                  n += len(data)
                  if n > limit:
                      raise OSError('too large')
              return status, b''.join(out)
          finally:
              shut(w)

      # ---------------------------------------------------------------------------
      # A channel: one source of video, any number of readers.
      # ---------------------------------------------------------------------------
      def clean_start(buf):
          """Where a new reader can start cleanly in MPEG-TS: at a program table (PAT) if one is near,
          or else at the first whole packet. 0 if it does not look like MPEG-TS."""
          end = len(buf) - 376
          i = buf.find(b'\x47')
          tries = 0
          while 0 <= i < end and tries < 2000:
              if buf[i + 188] == 0x47 and buf[i + 376] == 0x47:
                  for k in range(i, min(end, i + 188 * 3000), 188):
                      if buf[k] != 0x47:
                          break
                      if (buf[k + 1] & 0x1f) == 0 and buf[k + 2] == 0:
                          return k
                  return i
              i = buf.find(b'\x47', i + 1)
              tries += 1
          return 0

      class Reader:
          def __init__(self):
              self.q, self.n, self.wake, self.over, self.behind, self.w = deque(), 0, asyncio.Event(), False, None, None
              self.given = 0                             # what it was handed at once when it joined
          def push(self, data):
              self.q.append(data)
              self.n += len(data)
              self.wake.set()
          def end(self, drop=False):
              if drop:              # nothing more is sent to it, not even what was waiting, and its connection is closed
                  self.q.clear()
                  self.n = 0
                  if self.w is not None:
                      shut(self.w)
              self.over = True
              self.wake.set()

      class Channel:
          def __init__(self, key, source, provider):
              self.key, self.provider, self.readers = key, provider, set()
              self.timer = None                          # the linger, when one is running
              self.ring, self.kept = deque(), 0          # (time, bytes) of the newest video
              self.status, self.ready = None, asyncio.Event()   # 200 once video flows, or the refusal
              self.refusal, self.done = None, False
              self.total, self.marks = 0, deque()        # every byte the provider has sent, and (time, total) once a second for a minute
              self.born, self.burst = None, 0        # when the first video came, and how much the provider sent at once then
              self.asked = time.monotonic()
              self.task = asyncio.ensure_future(self.run(source))
              say('a channel was opened (%d open now)' % (len(CHANNELS) + 1))

          async def run(self, source):
              gen = source(self)
              try:
                  async for data in gen:
                      if self.done:
                          break
                      self.feed(data)
                  if self.status is None:
                      self.status = 502
              except Refused as e:
                  if self.status is None:
                      self.status, self.refusal = e.status, e
              except asyncio.CancelledError:
                  raise
              except asyncio.TimeoutError:
                  if self.status is None:
                      self.status = 504
                  say('a channel sent nothing for too long and was closed')
              except Exception as e:
                  if self.status is None:
                      self.status = 502
                  say('a channel ended: ' + type(e).__name__)
              finally:
                  try:
                      await gen.aclose()                # the provider's connection is closed here whatever happened
                  except BaseException:
                      pass
                  self.done = True
                  LAST_DROP[self.provider] = time.monotonic()
                  if CHANNELS.get(self.key) is self:
                      del CHANNELS[self.key]
                  say('a channel was closed (%d open now)' % len(CHANNELS))
                  self.ready.set()
                  for rd in self.readers:
                      rd.end()

          def feed(self, data):
              if self.status is None:
                  self.status = 200
                  self.ready.set()
              now = time.monotonic()
              if self.born is None:
                  self.born = now
                  say('a channel gave its first video %.1f s after it was asked for' % (now - self.asked))
              self.total += len(data)
              if not self.marks or now - self.marks[-1][0] >= 1:
                  self.marks.append((now, self.total))
                  while now - self.marks[0][0] > 60:
                      self.marks.popleft()
              if self.ring and now - self.ring[-1][0] > 4:
                  say('a provider sent nothing for %.0f s, then carried on' % (now - self.ring[-1][0]))
              if now - self.born < BURST_SECONDS:
                  self.burst += len(data)
              elif self.burst >= 0:
                  say('its opening burst was %d KB' % (self.burst // 1024))
                  self.burst = -self.burst               # (kept as a minus number once it has been measured and noted)
              self.ring.append((now, data))
              self.kept += len(data)
              speed = self.speed(now)
              keep = RING if speed is None else min(max(RING, int(speed * KEEP_SECONDS)), RING_MAX)
              while self.kept - len(self.ring[0][1]) >= keep:
                  self.kept -= len(self.ring.popleft()[1])
              for rd in list(self.readers):
                  if rd.n <= BEHIND + rd.given:           # (what a joiner was handed at the start is not "falling behind")
                      rd.behind = None
                  elif rd.behind is None:
                      rd.behind = now
                  if rd.n > FAR_BEHIND + rd.given or (rd.behind is not None and now - rd.behind > BEHIND_SECONDS):
                      # it cannot keep up; holding more for it would only use memory
                      say('a reader was too slow to keep up with its channel and was disconnected')
                      rd.end(drop=True)
                      self.readers.discard(rd)
                      self.drop_if_empty()
                  else:
                      rd.push(data)

          def speed(self, now):
              """Bytes of video a second; None in a channel's first seconds. It is measured over the last
              minute and over the last 15 seconds, and the larger is used, so that a channel whose bit
              rate has just gone up is not short-changed. A feed that has stopped is measured up to its
              last video, not up to now."""
              if len(self.marks) < 2 or now - self.marks[0][0] < 8:
                  return None
              end = now if now - self.ring[-1][0] < 15 else self.ring[-1][0]
              best = 0.0
              for at, total in (self.marks[0], next((m for m in reversed(self.marks) if end - m[0] >= 15), self.marks[0])):
                  if end - at >= 8:
                      best = max(best, (self.total - total) / (end - at))
              return best or None

          def join(self, first):
              """A new reader. The one who opens a channel gets it from its first byte; one who joins
              starts with the newest JOIN_SECONDS of video (see there), on a whole packet."""
              if self.timer is not None:                 # somebody is here after all: the linger is off
                  self.timer.cancel()
                  self.timer = None
              rd = Reader()
              if first:
                  for _, data in self.ring:
                      rd.push(data)
              elif self.ring:
                  speed = self.speed(time.monotonic())
                  want = self.kept if speed is None else int(speed * JOIN_SECONDS)   # (a channel just opened: all of it)
                  take, size, oldest = [], 0, self.ring[-1][0] - 2 * JOIN_SECONDS
                  for t, data in reversed(self.ring):
                      if size and (size + len(data) > want or t < oldest):   # (never more than 20 s of arrivals, whatever the speed says)
                          break
                      take.append(data)
                      size += len(data)
                  take.reverse()
                  head, n = b'', 0                      # only the beginning is copied, to find a clean place to start
                  while n < len(take) and len(head) < 262144:
                      head += take[n]
                      n += 1
                  if head:
                      rd.push(head[clean_start(head):])
                  for data in take[n:]:
                      rd.push(data)
                  rd.given = rd.n
                  say('a reader joined a channel and was given %d KB to start%s' % (rd.n // 1024, '' if speed is None else ' (%d s at %d KB a second)' % (JOIN_SECONDS, speed // 1024)))
              self.readers.add(rd)
              return rd

          def leave(self, rd):
              self.readers.discard(rd)
              if not self.readers and not self.done:   # the last one out
                  now, opened = time.monotonic(), LAST_OPEN.get(self.provider, (0, ''))
                  tuning = self.born is None or now - self.born < 5            # a player tunes in a channel's first seconds
                  moved = now - opened[0] < 1.0 and opened[1] != self.key      # the player has just gone to another channel
                  if tuning and not moved:
                      if self.timer is not None:
                          self.timer.cancel()
                      self.timer = asyncio.get_running_loop().call_later(LINGER, self.drop_if_empty)   # see LINGER
                  else:
                      self.drop()

          def drop_if_empty(self):
              if not self.readers:
                  self.drop()

          def drop(self):                              # let go of the provider's stream now
              if self.timer is not None:
                  self.timer.cancel()
                  self.timer = None
              if not self.done:
                  self.task.cancel()
                  self.done = True
                  LAST_DROP[self.provider] = time.monotonic()
                  if CHANNELS.get(self.key) is self:
                      del CHANNELS[self.key]

      CHANNELS = {}
      LAST_DROP = {}                                   # for each provider: when one of its streams was last let go
      LAST_OPEN = {}                                   # for each provider: when a channel was last opened, and which

      # ---- where a /ts channel's video comes from: the provider's own MPEG-TS stream
      def ts_source(target, agent):
          async def source(ch):
              again, sent, quiet = 0, 0, 0
              while True:
                  began, got, timed = time.monotonic(), False, False
                  # (asked for again, a provider gets a short while to answer and to send video: readers are waiting)
                  status, headers, r, w = await fetch(target, agent, wait=REOPEN_WAIT if again else HEADER_WAIT)
                  try:
                      if status not in (200, 206):
                          say('a provider answered %d when asked for a channel' % status)
                          text = b''
                          try:      # what the provider said, to pass on to the player
                              async for data in body(r, headers, 5):
                                  text += data
                                  if len(text) > 65536:
                                      break
                          except (OSError, asyncio.TimeoutError):
                              pass
                          raise Refused(status, headers.get('content-type', ''), text[:65536])
                      head = b'' if again else None     # a stream opened again is joined on at a whole packet
                      pieces, started = body(r, headers).__aiter__(), False
                      while True:
                          try:
                              if again and not started:
                                  data = await asyncio.wait_for(pieces.__anext__(), 2 * REOPEN_WAIT)
                              else:
                                  data = await pieces.__anext__()
                          except StopAsyncIteration:
                              data = b''
                          started = True
                          if head is not None:
                              head += data
                              if data and len(head) < 16384:
                                  continue
                              data, head = head[clean_start(head):], None
                              if data and sent % 188:   # the old stream stopped part-way through a packet: fill it out,
                                  fill = b'\xff' * (188 - sent % 188)   # so the new stream's packets line up with the old
                                  sent += len(fill)
                                  yield fill
                          if not data:
                              break
                          got = True
                          sent += len(data)
                          yield data
                  except (OSError, asyncio.TimeoutError) as e:
                      if not got:
                          raise
                      timed = isinstance(e, asyncio.TimeoutError)
                  finally:
                      shut(w)
                  # The provider ended the stream. If anyone is still watching, ask for it again.
                  # Not for a stream that ended within REOPEN_AFTER seconds (a short clip, a refusal dressed
                  # up as video), and not when another channel of this provider was opened in the last 15
                  # seconds: on a one-connection account that is why this one ended, and asking again would
                  # end the other.
                  ran, opened = time.monotonic() - began, LAST_OPEN.get(ch.provider, (0, ''))
                  # A stream that sent under REOPEN_AFTER seconds of video and then went silent, twice in a row:
                  # the channel is dead, so the readers are let go and the provider is not asked again.
                  quiet = quiet + 1 if timed and ran < QUIET_WAIT + REOPEN_AFTER else 0
                  if quiet >= 2:
                      say('a channel went silent twice in a row; letting its readers go')
                      return
                  if not got or not ch.readers or ran < REOPEN_AFTER or (opened[0] > max(began, time.monotonic() - 15) and opened[1] != ch.key):
                      return
                  again = 1
                  say('a provider ended a channel that was being watched; asking for it again')
                  await asyncio.sleep(DROP_GAP)
          return source

      # ---- where a /hls channel's video comes from: the channel's HLS pieces, one after another.
      # Everything is fetched from nginx in this stack, at the same addresses the players use, so the
      # reader shares the players' one provider session (see "session memory" in the nginx notes).
      class NotHls(Exception):
          pass

      def read_playlist(text):
          if '#EXTM3U' not in text:
              raise NotHls()
          seq, target, pieces, variants, rate, ended = 0, 4.0, [], [], None, False
          for line in text.splitlines():
              line = line.strip()
              if line.startswith('#EXT-X-MEDIA-SEQUENCE:'):
                  seq = int(re.sub(r'\D', '', line[22:]) or 0)
              elif line.startswith('#EXT-X-TARGETDURATION:'):
                  target = float(re.sub(r'[^0-9.]', '', line[22:]) or 4)
              elif line.startswith('#EXT-X-STREAM-INF'):
                  m = re.search(r'[:,]BANDWIDTH=(\d+)', line)
                  rate = int(m.group(1)) if m else 0
              elif line.startswith('#EXT-X-ENDLIST'):
                  ended = True
              elif line.startswith(('#EXT-X-MAP', '#EXT-X-BYTERANGE')) or (line.startswith('#EXT-X-KEY') and 'METHOD=NONE' not in line):
                  raise NotHls()                        # locked, or not whole MPEG-TS files: cannot be joined end to end
              elif line and not line.startswith('#'):
                  if rate is not None:
                      variants.append((rate, line))
                      rate = None
                  else:
                      pieces.append(line)
          best = max(variants, key=lambda v: v[0])[1] if variants else None   # the best quality on offer
          return seq, target, pieces, best, ended

      def hls_source(start, agent):
          home = urlsplit(start).netloc
          def inside(url):                              # the playlist's addresses must all be this proxy's own
              if urlsplit(url).netloc != home:
                  raise NotHls()
              return url
          def name(url):
              return urlsplit(url).path

          async def look():
              # Always from the channel's own address, so the session memory is kept alive. (For a channel
              # offered as a list of qualities that is two requests a look where a player makes one.)
              url = start
              for _ in range(4):
                  status, data = await get(url, agent, 2 * 1024 * 1024)
                  if status != 200:
                      raise Refused(status)
                  seq, target, pieces, variant, ended = read_playlist(data.decode('utf-8', 'replace'))
                  if not variant:
                      return url, seq, target, pieces, ended
                  url = inside(urljoin(url, variant))
              raise NotHls()

          async def source(ch):
              last, misses, lost, sent, fresh = None, 0, 0, deque(maxlen=60), time.monotonic()
              while True:
                  try:
                      url, seq, target, pieces, ended = await look()
                      misses = 0
                  except (NotHls, Refused, OSError, ValueError, asyncio.TimeoutError) as e:
                      # no playlist this time (a refusal, an error page, a playlist that cannot be joined
                      # end to end): ask again, as a player would
                      misses += 1
                      refused = e.status if isinstance(e, Refused) else 0
                      if misses > (8 if last is not None else 3):   # the playlist could not be had: 4 tries at the start, 9 once playing
                          raise Refused(refused or 502)
                      await asyncio.sleep(2 if last is not None else 1)
                      continue
                  top = seq + len(pieces) - 1
                  afresh = last is not None and (top < last or seq > last + 1)
                  if last is None or afresh:
                      last = max(seq - 1, top - 3)      # joining, or the numbering started afresh: start near the newest video
                  new = [(seq + i, p) for i, p in enumerate(pieces) if seq + i > last]
                  for number, piece in new:
                      last = number
                      try:
                          address = inside(urljoin(url, piece))
                      except NotHls:                    # a line that does not lead back to this proxy: not fetched
                          lost += 1
                          continue
                      if afresh and name(address) in sent:   # already passed on, under its old number
                          continue
                      data = None
                      for _ in range(2):
                          try:
                              status, data = await get(address, agent)
                              if status == 200 and data:
                                  break
                              data = None
                          except (OSError, ValueError, asyncio.TimeoutError):
                              data = None
                      if data is None:
                          lost += 1
                          say('a piece of video could not be fetched and is missing from a stream')
                          if lost >= 3:
                              raise Refused(502)
                          continue
                      lost = 0
                      sent.append(name(address))
                      fresh = time.monotonic()
                      for i in range(0, len(data), 65536):
                          yield data[i:i + 65536]
                  if not new and (ended or time.monotonic() - fresh > max(30, 6 * target)):
                      return                            # the channel's playlist has finished, or stopped moving
                  # ask again at the pace a player would
                  await asyncio.sleep(max(0.5, target if new else target / 2))
          return source

      # ---------------------------------------------------------------------------
      # Requests from nginx
      # ---------------------------------------------------------------------------
      # What a player sent that is not passed on to the provider: what belongs to one connection, and this proxy's own notes.
      SKIP = re.compile(r'^(host|connection|keep-alive|proxy-.*|te|trailer|transfer-encoding|upgrade|content-length'
                        r'|range|if-range|accept-encoding|x-relay-.*|x-hls-.*|x-real-ip|x-forwarded-.*)$$', re.I)
      HEAD_OK = b'HTTP/1.1 200 OK\r\nContent-Type: video/mp2t\r\nConnection: close\r\n\r\n'
      OPEN = set()                                      # every connection from nginx that is open now

      def answer(w, status, text=b'', kind='text/plain', more=''):
          try:
              reason = http.HTTPStatus(status).phrase
          except ValueError:
              reason = 'Error'
          if isinstance(text, str):
              text = text.encode()
          w.write(('HTTP/1.1 %d %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n%sConnection: close\r\n\r\n'
                   % (status, reason, kind or 'text/plain', len(text), more)).encode('latin-1', 'replace') + text)

      def channel_key(path, provider, uri):
          """One channel has one name here, whichever form of its address a player uses
          (/live/user/pass/1.ts, /user/pass/1.ts, /user/pass/1)."""
          parts = [p for p in uri.split('?')[0].split('/') if p]
          if parts and parts[0] == 'live':
              parts = parts[1:]
          if parts and parts[-1].endswith('.ts'):
              parts[-1] = parts[-1][:-3]
          return path + ' ' + provider + ' ' + '/'.join(parts)

      # ---------------------------------------------------------------------------
      # HLS playlists: every address in one is pointed back at this proxy
      # ---------------------------------------------------------------------------
      # nginx fetches the playlist from the provider exactly as before (same session, same connections).
      # Asked from the address RAW below, it hands the playlist over as the provider wrote it, with three
      # notes saying where this playlist's addresses lead:
      #   X-Pl-Cdn   this proxy's address for the server the playlist came from     (for "/path")
      #   X-Pl-Rel   the same, for an address written without http or https         (for "//server/path")
      #   X-Pl-Up    this proxy's address for the playlist's own folder, one level up (for "../path")
      # A full address (http://server/path) needs no note. An address with none of these beginnings is
      # next to the playlist itself and is left as it is.
      RAW = '127.0.0.2'
      URI_IN_TAG = re.compile(r'(URI=")([^"]*)(")', re.I)

      def rewrite_playlist(text, cdn, rel, up):
          def move(u):
              start = u[:8].lower()
              if start.startswith('https://'):
                  return '/__cdn/https/' + u[8:]
              if start.startswith('http://'):
                  return '/__cdn/http/' + u[7:]
              if u.startswith('//'):
                  return rel + '/' + u[2:]
              if u.startswith('/'):
                  return cdn + u
              if u.startswith('../'):
                  return up + u[3:]
              return u
          def in_tags(line):
              return URI_IN_TAG.sub(lambda m: m.group(1) + move(m.group(2)) + m.group(3), line)
          out = []
          for number, line in enumerate(text.split('\n')):
              core = line.lstrip(' \t')
              if number == 0 or not core.strip():
                  out.append(line)
              elif core.startswith('#'):                # a tag: addresses sit in URI="..."
                  out.append(in_tags(line))
              else:                                     # a line that is an address
                  moved = move(core)
                  out.append(in_tags(moved if moved != core else line))   # (a line that is not moved is left exactly as it was)
          return '\n'.join(out)

      # What is passed between player and server on a playlist, either way: everything except what belongs
      # to one connection and this proxy's own notes.
      PL_SKIP = re.compile(r'^(host|connection|keep-alive|proxy-.*|te|trailer|transfer-encoding|upgrade|content-length'
                           r'|range|if-range|content-range|accept-ranges|x-accel-.*|server|date|x-relay-.*|x-hls-.*|x-pl-.*)$$', re.I)
      HLS_SEEN = {}                                     # channels whose playlist a player has just asked for (for the status page)

      async def playlist(w, h, raw):
          port, orig = h.get('x-relay-port', ''), h.get('x-relay-orig', '')
          if not port.isdigit() or not orig.startswith('/'):
              return answer(w, 400)
          asked = [nv for nv in raw if not PL_SKIP.match(nv[0])]
          status, headers, r, up = await fetch('http://127.0.0.1:%s%s' % (port, orig), asked, guard=False, source=RAW, follow=False, wait=PLAYLIST_WAIT)
          if headers.get('content-encoding', '').strip().lower() not in ('', 'identity'):
              shut(up)
              raise OSError('compressed')               # it cannot be read here: with no answer from here, nginx serves it by its own rules
          try:
              data = b''
              async for piece in body(r, headers, PLAYLIST_WAIT):
                  data += piece
                  # Not a playlist after all (some providers serve other things at a playlist's address), or
                  # too big to be handled here: step aside. With no answer from here, nginx serves it itself.
                  if len(data) > 1024 * 1024 or (status == 200 and len(data) >= 1024 and b'#EXTM' not in data[:1024]):
                      raise OSError('not a playlist for the relay')
          finally:
              shut(up)
          if b'#EXTM' in data[:1024]:
              if data.count(b'\n') > 20000:
                  raise OSError('too many lines')
              # latin-1 both ways, so every byte outside an address comes out as it went in
              text = rewrite_playlist(data.decode('latin-1'), headers.get('x-pl-cdn', ''), headers.get('x-pl-rel', '/__cdn/https'),
                                      headers.get('x-pl-up', '../'))
              if status == 200:
                  m = re.match(r'^/__ch(/.+?\.m3u8)/', orig)
                  if m:
                      HLS_SEEN[port + m.group(1)] = time.monotonic()
                      if len(HLS_SEEN) > 300:
                          for k in [k for k, t in HLS_SEEN.items() if time.monotonic() - t > 30]:
                              del HLS_SEEN[k]
              data = text.encode('latin-1')
          try:
              reason = http.HTTPStatus(status).phrase
          except ValueError:
              reason = 'Status'
          lines = ['HTTP/1.1 %d %s' % (status, reason)] + ['%s: %s' % nv for nv in headers.all if not PL_SKIP.match(nv[0])]
          lines += ['Content-Length: %d' % len(data), 'Connection: close', '', '']
          w.write('\r\n'.join(lines).encode('latin-1', 'replace') + data)

      # ---------------------------------------------------------------------------
      # The status page
      # ---------------------------------------------------------------------------
      KEPT = {}                                         # answers from outside, kept for a while so the page is quick

      ASKING = {}                                       # lookups under way, so that many page views share one

      async def kept(name, seconds, make):
          """make()'s answer, asked for at most once every `seconds`; None if it takes too long or fails."""
          had = KEPT.get(name)
          if had and time.monotonic() - had[0] < (seconds if had[1] is not None else 60):
              return had[1]
          async def ask():
              try:
                  value = await asyncio.wait_for(make(), 6)
              except Exception:
                  value = None
              KEPT[name] = (time.monotonic(), value)
              return value
          if name not in ASKING:
              ASKING[name] = asyncio.ensure_future(ask())
              ASKING[name].add_done_callback(lambda f: ASKING.pop(name, None))
          return await asyncio.shield(ASKING[name])

      async def where_am_i():                           # what the internet sees: asked through the VPN
          async def ask():
              status, headers, r, w = await fetch('https://ipinfo.io/json', 'curl/8', guard=False, tls=CHECKED)
              try:
                  text = b''
                  async for data in body(r, headers, 6):
                      text += data
                  return json.loads(text) if status == 200 else None
              finally:
                  shut(w)
          return await kept('where', 300, ask)

      async def account(port, user, password):          # the provider's own account page, asked through nginx with the easy login
          async def ask():
              url = 'http://127.0.0.1:%s/player_api.php?username=%s&password=%s' % (port, quote(user, safe=''), quote(password, safe=''))
              status, data = await get(url, '', 1 << 20)
              if status == 502:                         # most often a name lookup that failed for a moment: asked once more
                  await asyncio.sleep(0.5)
                  status, data = await get(url, '', 1 << 20)
              if status != 200:
                  return {'answer': status}
              info = json.loads(data).get('user_info') or {}
              return {'answer': 200, 'status': info.get('status'), 'auth': info.get('auth'), 'exp': info.get('exp_date'),
                      'used': info.get('active_cons'), 'max': info.get('max_connections')}
          # The account page is asked for when somebody looks at the status page, at most once every
          # ACCOUNT_EVERY seconds, and the page waits a few seconds for the answer, so that what it shows
          # (connections in use, above all) is how things are now. If the provider is slow for a moment the
          # last good answer is shown instead, with its age beside it, and not "no answer". After a failure
          # the provider is asked less and less often (2, 4, 8, at most 15 minutes), so that a provider in
          # trouble is left alone.
          st = ACCOUNTS.setdefault(port, {'good': None, 'at': 0.0, 'tried': -1e9, 'fails': 0, 'task': None})
          now = time.monotonic()
          async def refresh():
              try:
                  value = await asyncio.wait_for(ask(), 20)
              except Exception:
                  value = None
              if value and value.get('answer') == 200:
                  st['good'], st['at'], st['fails'] = value, time.monotonic(), 0
                  st.pop('last', None)
              else:
                  st['fails'] += 1
                  st['last'] = value
              st['task'] = None
          if st['task'] is None and now - st['tried'] >= (ACCOUNT_EVERY if not st['fails'] else min(60 * 2 ** st['fails'], 900)):
              st['tried'] = now
              st['task'] = asyncio.ensure_future(refresh())
          if st['task'] is not None:                         # an answer is on its way: wait a little for it
              await asyncio.wait({st['task']}, timeout=6)
          if st['good'] is not None and time.monotonic() - st['at'] < 900:
              return dict(st['good'], age=time.monotonic() - st['at'])
          return st.get('last')

      ACCOUNTS = {}
      ACCOUNT_EVERY = 15

      STYLE = """
      :root{--bg:#f5f6f8;--card:#fff;--ink:#16181d;--soft:#5b6270;--line:#e6e8ee;--chip:#eef0f5;--good:#12805c;--goodbg:#e7f6ef;
      --bad:#c0362c;--badbg:#fdecea;--warn:#8a5a00;--warnbg:#fff4d6;--link:#2457d6;--shadow:0 1px 2px rgba(16,24,40,.06),0 4px 16px rgba(16,24,40,.06)}
      @media (prefers-color-scheme:dark){:root{--bg:#0f1115;--card:#181b22;--ink:#eceef2;--soft:#9aa3b2;--line:#272b35;--chip:#232733;
      --good:#4ad295;--goodbg:#11281f;--bad:#ff8a80;--badbg:#2b1614;--warn:#ffcf66;--warnbg:#2a2210;--link:#8ab0ff;--shadow:0 1px 2px rgba(0,0,0,.4)}}
      *{box-sizing:border-box}html{-webkit-text-size-adjust:100%}
      body{margin:0;background:var(--bg);color:var(--ink);font:17px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
      main{max-width:860px;margin:0 auto;padding:28px 18px 48px}
      header{display:flex;align-items:center;gap:12px;margin:0 0 18px}
      header .dot{width:14px;height:14px;border-radius:50%;background:var(--good);box-shadow:0 0 0 5px var(--goodbg)}
      header.down .dot{background:var(--bad);box-shadow:0 0 0 5px var(--badbg)}
      h1{font-size:24px;letter-spacing:-.01em;margin:0;font-weight:700}
      h2{font-size:13px;letter-spacing:.08em;text-transform:uppercase;color:var(--soft);margin:0 0 12px;font-weight:700}
      .card{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:20px 22px;margin:14px 0;box-shadow:var(--shadow)}
      .banner{border:0;font-size:20px;font-weight:700;display:flex;gap:12px;align-items:flex-start}
      .banner small{display:block;font-size:16px;font-weight:500;margin-top:2px}
      .good{background:var(--goodbg);color:var(--good)}.bad{background:var(--badbg);color:var(--bad)}.warn{background:var(--warnbg);color:var(--warn)}
      .bad ul,.warn ul{font-size:16px;font-weight:500;margin:8px 0 0;padding-left:20px}
      .pair{display:grid;grid-template-columns:max-content 1fr;gap:6px 18px;margin:0}
      .pair dt{color:var(--soft)}.pair dd{margin:0;font-weight:600}
      .provider{border-top:1px solid var(--line);padding:16px 0}.provider:first-of-type{border-top:0;padding-top:2px}.provider:last-of-type{padding-bottom:2px}
      .provider .top{display:flex;flex-wrap:wrap;align-items:center;gap:8px 10px;margin-bottom:8px}
      .provider .name{font-size:19px;font-weight:700;margin-right:4px}
      .pill{display:inline-block;font-size:13px;font-weight:700;padding:3px 10px;border-radius:999px;background:var(--chip);color:var(--soft);white-space:nowrap}
      .pill.good{background:var(--goodbg);color:var(--good)}.pill.bad{background:var(--badbg);color:var(--bad)}.pill.warn{background:var(--warnbg);color:var(--warn)}
      code{font:14.5px/1.4 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;background:var(--chip);padding:3px 8px;border-radius:7px;word-break:break-all}
      .facts{display:flex;flex-wrap:wrap;gap:6px 22px;color:var(--soft);font-size:15.5px;margin-top:8px}.facts b{color:var(--ink);font-weight:600}
      p{margin:10px 0 0}.note{color:var(--soft);font-size:15px}ul{margin:8px 0 0;padding-left:20px}li{margin:7px 0}
      a{color:var(--link);text-decoration:none}a:hover{text-decoration:underline}
      footer{color:var(--soft);font-size:14px;text-align:center;margin-top:22px}
      """

      def esc(text):
          return html.escape(str(text), quote=True)

      async def status_page(w, h):
          port = h.get('x-relay-port', '')
          host = (h.get('host', '') or 'this-machine').rsplit(':', 1)[0] if port.isdigit() else 'this-machine'
          info = {'problems': ['The proxy could not read its own start-up notes. Look at the log: docker logs iptv-nginx'], 'providers': []}
          if port.isdigit():
              try:
                  status, data = await asyncio.wait_for(get('http://127.0.0.1:%s/__status.json' % port, '', 1 << 20), 5)
                  if status == 200:
                      info = json.loads(data)
              except Exception:
                  pass
          problems = [str(p) for p in info.get('problems') or []]
          good = [p for p in info.get('providers') or [] if p.get('ok')]
          skipped = [p for p in info.get('providers') or [] if not p.get('ok')]
          working = not problems and bool(good)
          user, password = h.get('x-relay-user', ''), h.get('x-relay-pass', '')
          where, accounts = await asyncio.gather(where_am_i(), asyncio.gather(*[account(str(p.get('port')), user, password) for p in good]))

          o = ['<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">',
               '<meta name="color-scheme" content="light dark"><meta http-equiv="refresh" content="30"><title>IPTV proxy status</title><style>', STYLE, '</style></head><body><main>',
               '<header class="%s"><span class="dot"></span><h1>IPTV proxy status</h1></header>' % ('' if working else 'down')]
          if working:
              o.append('<div class="card banner good"><div>&#9989;</div><div>The proxy is working.<small>%d provider%s loaded.</small></div></div>'
                       % (len(good), ' is' if len(good) == 1 else 's are'))
          else:
              o.append('<div class="card banner bad"><div>&#10060;</div><div>The proxy is NOT working yet.<ul>')
              o += ['<li>%s</li>' % esc(p) for p in problems]
              if not problems and not good:
                  o.append('<li>No provider is set up. Open your settings file (<code>.env</code>) and add the five lines for a provider. The file itself shows how.</li>')
              o.append('</ul><small>After you fix the settings file, start the stack again and reload this page.</small></div></div>')
          if skipped:
              o.append('<div class="card warn"><h2 style="color:inherit">&#9888;&#65039; These providers were left out</h2><ul>')
              o += ['<li><b>%s</b>: %s</li>' % (esc(p.get('name', '')), esc(p.get('why', ''))) for p in skipped]
              o.append('</ul><p>Fix the line in your settings file (<code>.env</code>) and start the stack again.</p></div>')

          o.append('<div class="card"><h2>Your VPN</h2>')
          if where and where.get('ip'):
              place = ', '.join(str(where[k]) for k in ('city', 'region', 'country') if where.get(k))
              o.append('<dl class="pair"><dt>The internet sees you as</dt><dd>%s</dd><dt>Location</dt><dd>%s</dd>%s</dl>'
                       % (esc(where['ip']), esc(place), '<dt>Network</dt><dd>%s</dd>' % esc(where['org']) if where.get('org') else ''))
              o.append('<p class="note">This should be your VPN, not your home. If you see your own town and your own internet company here, stop and check your VPN settings.</p>')
          else:
              o.append('The proxy could not look up its public address just now. Reload this page in a minute.'
                       '<p class="note">The VPN container still blocks all traffic when the VPN is down, so nothing leaves from your home address.</p>')
          o.append('</div>')

          o.append('<div class="card"><h2>Your providers</h2>')
          if not good:
              o.append('None loaded.')
          for p, a in zip(good, accounts):
              pill, facts = '<span class="pill warn">no answer from the provider</span>', []
              if a and a.get('answer') == 200:
                  state = str(a.get('status') or '')
                  if str(a.get('auth')) == '0':
                      pill = '<span class="pill bad">login refused by the provider</span>'
                  elif state:
                      pill = '<span class="pill %s">%s</span>' % ('good' if state.lower() == 'active' else 'warn', esc(state))
                  else:
                      pill = '<span class="pill">answered</span>'
                  try:
                      facts.append('Expires <b>%s</b>' % time.strftime('%Y-%m-%d', time.gmtime(int(a['exp']))) if a.get('exp') else 'No end date')
                  except (ValueError, TypeError, OverflowError, OSError):
                      pass
                  if a.get('max') is not None:
                      facts.append('In use <b>%s of %s</b>' % (esc('?' if a.get('used') is None else a['used']), esc(a['max'])))
                  age = a.get('age', 0)
                  facts.append('Checked <b>%s</b>' % ('just now' if age < 5 else '%d s ago' % age if age < 120 else '%d min ago' % (age // 60)))
              elif a and a.get('answer'):
                  pill = '<span class="pill bad">the provider answered %s</span>' % esc(a['answer'])
              o.append('<div class="provider"><div class="top"><span class="name">%s</span>%s<span class="pill">%s</span></div>'
                       '<div>Type this into your player: <code>http://%s:%s</code></div><div class="facts">%s</div></div>'
                       % (esc(p.get('name', '')), pill, 'MPEG-TS' if p.get('mode') == 'ts' else 'HLS', esc(host), esc(p.get('port', '')),
                          ''.join('<span>%s</span>' % f for f in facts)))
          if good:
              o.append('<p class="note">In your player, add an Xtream login with that address and the easy username and password from your settings file (<code>PLAYER_USER</code> and <code>PLAYER_PASSWORD</code>).</p>')
          o.append('</div>')

          o.append('<div class="card"><h2>The Live TV setting</h2>Each provider has one line in your settings file that says how your players watch its live channels.'
                   '<ul><li><code>NAME_LIVE_TV=hls</code> Your players use <b>HLS</b>. Everyone on a channel shares one connection to the provider, so you can watch and record the same channel at once. A recorder that asks for MPEG-TS is given the same video as your players.</li>'
                   '<li><code>NAME_LIVE_TV=ts</code> Your players use <b>MPEG-TS</b>. You get the provider&#39;s own stream, and everyone on a channel shares one connection to the provider. Do not open a channel as HLS while it is playing or recording as MPEG-TS: most providers allow one kind at a time per channel, and the two will cut each other off.</li></ul>'
                   '<p class="note">Pick the one that matches the live format chosen in your players and in your playlist manager if you use one. They have to agree. If a provider only offers one of the two, use that one.</p></div>')

          now = time.monotonic()
          for k in [k for k, t in HLS_SEEN.items() if now - t > 30]:
              del HLS_SEEN[k]
          hls, ts = len(HLS_SEEN), sum(1 for c in CHANNELS.values() if c.key.startswith('/ts '))
          readers = sum(len(c.readers) for c in CHANNELS.values() if c.key.startswith('/ts '))
          recorders = sum(len(c.readers) for c in CHANNELS.values() if c.key.startswith('/hls '))
          playing = []
          if hls:
              playing.append('<b>%d</b> channel%s as HLS' % (hls, '' if hls == 1 else 's'))
          if ts:
              playing.append('<b>%d</b> channel%s as MPEG-TS, shared by <b>%d</b> reader%s' % (ts, '' if ts == 1 else 's', readers, '' if readers == 1 else 's'))
          if recorders:
              playing.append('<b>%d</b> MPEG-TS reader%s being fed from HLS' % (recorders, '' if recorders == 1 else 's'))
          o.append('<div class="card"><h2>Playing right now</h2>%s<p class="note">Live channels only. Movies, series and catch-up are not counted.</p></div>'
                   % ('Nothing.' if not playing else '<br>'.join(playing)))

          o.append('<div class="card"><h2>Still stuck?</h2>Each of the three containers keeps a log. Run the command on the machine this stack runs on.<ul>'
                   '<li><code>docker logs iptv-nginx</code><br>The proxy. At the top it lists every provider it found, and any it left out and why. After that, one line for every request.</li>'
                   '<li><code>docker logs iptv-relay</code><br>The Python relay. It says when a live channel is opened and closed, how long the provider took to send the first video, and anything that went wrong with a channel.</li>'
                   '<li><code>docker logs iptv-vpn</code><br>The VPN. It says whether the tunnel is up and where it comes out.</li>'
                   '<li>The instructions: <a href="https://github.com/mackid1993/iptv-vpn-proxy">github.com/mackid1993/iptv-vpn-proxy</a></li>'
                   '</ul></div><footer>IPTV VPN proxy</footer></main></body></html>')
          answer(w, 200 if working else 503, ''.join(o), 'text/html; charset=utf-8', 'Cache-Control: no-store\r\n')

      async def handle(r, w):
          rd = ch = None
          OPEN.add(w)
          try:
              line = (await asyncio.wait_for(r.readline(), 10)).decode('latin-1').split()
              h, sent, raw = {}, [], []
              for _ in range(HEADER_LINES):
                  x = (await asyncio.wait_for(r.readline(), 10)).decode('latin-1')
                  if x in ('\r\n', '\n', ''):
                      break
                  name, _, value = x.partition(':')
                  h[name.strip().lower()] = value.strip()
                  raw.append((name.strip(), value.strip()))
                  if not SKIP.match(name.strip()):       # the player's own headers, in its order, for the provider
                      sent.append((name.strip(), re.sub(r'[^\x20-\x7e]', '', value.strip())))
              if len(line) < 2:
                  return
              method, path = line[0], line[1].split('?')[0]
              if path == '/healthz':
                  return answer(w, 200, 'ok\n')
              if path == '/status':
                  return await status_page(w, h)
              if path == '/pl':
                  return await playlist(w, h, raw)
              if path == '/stat':
                  return answer(w, 200, '%d %d\n' % (len(CHANNELS), sum(len(c.readers) for c in CHANNELS.values())))
              provider, uri = h.get('x-relay-provider', '').rstrip('/'), h.get('x-relay-uri', '')
              if path not in ('/ts', '/hls') or provider not in PROVIDERS or not uri.startswith('/'):
                  return answer(w, 403, 'Not a request from the proxy.\n')
              if method == 'HEAD':                      # always answered here with 200: asking the provider would open a connection
                  w.write(HEAD_OK)
                  return
              if method != 'GET':
                  return answer(w, 400)
              agent = re.sub(r'[^\x20-\x7e]', '', h.get('user-agent', ''))
              target = provider + uri
              key = channel_key(path, provider, uri)
              ch = CHANNELS.get(key)
              first = ch is None
              if first:
                  # Changing channel. A player hangs up on the old channel and asks for the new one at
                  # almost the same moment, and the two can arrive here in either order. So, for this
                  # provider: channels nobody is reading are let go now; if one of its channels is still
                  # being read, a hang-up that is just behind this request is given a moment to arrive;
                  # and the provider is given DROP_GAP to see a hang-up before this channel is asked for.
                  LAST_OPEN[provider] = (time.monotonic(), key)
                  mine = [c for c in CHANNELS.values() if c.provider == provider]
                  for other in mine:
                      other.drop_if_empty()
                  if any(not c.done for c in mine):
                      await asyncio.sleep(0.03)
                      for other in mine:
                          other.drop_if_empty()
                  wait = LAST_DROP.get(provider, 0) + DROP_GAP - time.monotonic()
                  if wait > 0:
                      await asyncio.sleep(wait)
                  if r.at_eof():                        # the player has already moved on: nothing is opened for it
                      return
                  for other in CHANNELS.values():       # and once more, for hang-ups that came during the waits
                      if other.provider == provider and other.key != key:
                          other.drop_if_empty()
                  ch = CHANNELS.get(key)
                  first = ch is None
                  if first:
                      source = ts_source(target, sent)
                      if path == '/hls':
                          orig, port = h.get('x-relay-orig', ''), h.get('x-relay-port', '')
                          if not orig.startswith('/') or not port.isdigit():
                              return answer(w, 400)
                          q = orig.find('?')
                          plain, query = (orig, '') if q < 0 else (orig[:q], orig[q:])
                          plain = plain[:-3] if plain.endswith('.ts') else plain
                          source = hls_source('http://127.0.0.1:%s/__ch%s.m3u8/__index.m3u8%s' % (port, plain, query), agent)
                      ch = CHANNELS[key] = Channel(key, source, provider)
              if sum(len(c.readers) for c in CHANNELS.values()) >= READERS_MAX:
                  if first:
                      ch.drop()
                  ch = None
                  return answer(w, 503, 'Too many readers at once.\n')
              rd = ch.join(first)
              rd.w = w
              came, why = time.monotonic(), 'the player hung up'
              gone = asyncio.ensure_future(r.read(1))   # completes when nginx (so the player) hangs up
              gone.add_done_callback(lambda f: f.cancelled() or f.exception())   # a reset here is expected; keep it out of the log
              try:
                  wait = asyncio.ensure_future(ch.ready.wait())
                  await asyncio.wait({wait, gone}, timeout=START_WAIT, return_when=asyncio.FIRST_COMPLETED)
                  wait.cancel()
                  if gone.done():
                      return
                  if ch.status != 200:
                      e = ch.refusal
                      if e is not None and 400 <= e.status < 600:   # the provider's own answer, passed on
                          return answer(w, e.status, e.text, e.kind)
                      return answer(w, 504 if ch.status is None else ch.status if 400 <= ch.status < 600 else 502)
                  w.write(HEAD_OK)
                  while True:
                      if rd.q:
                          data = rd.q.popleft()
                          rd.n -= len(data)
                          rd.given = max(0, rd.given - len(data))   # (the join's allowance is used up as the join is sent)
                          w.write(data)
                          drain = asyncio.ensure_future(w.drain())
                          await asyncio.wait({drain, gone}, return_when=asyncio.FIRST_COMPLETED)
                          if gone.done():
                              drain.cancel()
                              return
                          drain.result()
                          continue
                      if rd.over:
                          why = 'its channel ended'
                          return
                      rd.wake.clear()
                      if rd.q or rd.over:
                          continue
                      wake = asyncio.ensure_future(rd.wake.wait())
                      await asyncio.wait({wake, gone}, return_when=asyncio.FIRST_COMPLETED)
                      wake.cancel()
                      if gone.done():
                          return
              except OSError:
                  why = 'the line to the player broke'
                  raise
              finally:
                  gone.cancel()
                  say('a reader left after %d s: %s (%d still on its channel)' % (time.monotonic() - came, why, max(0, len(ch.readers) - 1)))
          except (OSError, ValueError, asyncio.TimeoutError, asyncio.IncompleteReadError):
              pass
          except Exception as e:
              say('a request failed: ' + type(e).__name__)
          finally:
              OPEN.discard(w)
              if ch is not None and rd is not None:
                  ch.leave(rd)
              try:
                  w.close()
              except Exception:
                  pass

      async def main():
          server = await asyncio.start_server(handle, '127.0.0.1', PORT, limit=1 << 16)
          say('ready, for %d provider%s' % (len(PROVIDERS), '' if len(PROVIDERS) == 1 else 's'))
          stop = asyncio.Event()                        # stop when told to (this program is the container's first process)
          for sig in (signal.SIGTERM, signal.SIGINT):
              asyncio.get_running_loop().add_signal_handler(sig, stop.set)
          await stop.wait()
          server.close()                                # stopping: let go of every stream at once
          for ch in list(CHANNELS.values()):
              ch.task.cancel()
          for w in list(OPEN):
              shut(w)
          await asyncio.sleep(0.1)

      if __name__ == '__main__':
          asyncio.run(main())
```

</details>

## Everyday use

**Add a provider.** Add its five lines to `.env`, using a name of your choice
in capitals and a free port inside `PROXY_PORTS`:

```
SPORTS_URL=https://provider.example
SPORTS_PORT=8096
SPORTS_USER=the-username-they-gave-you
SPORTS_PASSWORD='the-p@ssword-they-gave-you'
SPORTS_LIVE_TV=hls
```

Then run `docker compose up -d`. Only the proxy and the relay restart; the VPN
stays up.

**Pick the live format.** `NAME_LIVE_TV` says how your players watch that
provider's live channels. There is no default, and a provider without the
line is left out.

- `hls`: your players use HLS. Everyone watching a channel shares one
  connection to the provider. A recorder that asks for MPEG-TS is given the
  same video as your players, so you can watch and record a channel at once.
- `ts`: your players use MPEG-TS. Everyone on a channel shares one connection
  to the provider and gets the provider's own stream, so you can watch and
  record a channel at once. Do not open a channel as HLS while it is playing
  or recording as MPEG-TS: most providers allow one kind at a time per
  channel.

Use the format chosen in your players, and in your playlist manager if you
have one. They all have to agree. If a provider offers only one of the two,
use that one. If it offers both and you have no reason to prefer one, use
`hls` (see [Trade-offs and things to know](#trade-offs-and-things-to-know)).

**Remove a provider.** Delete its five lines and run `docker compose up -d`.

**Logins with odd characters.** Provider logins can contain anything. You do
not get to choose them, so the proxy takes whatever you were given. One rule
for the `.env` file: if a username or password has anything besides letters
and digits, put it between single quotes. Without them, a `$` or `#` is read
as something else and part of the login is silently lost.

**More providers than ports.** Widen `PROXY_PORTS` (for example `8095-8099`)
and run `docker compose up -d`. This one restarts the VPN too.

**See what is happening.** The status page
(`http://<the machine's address>:<any provider's port>/status`) shows what is
set up, where the VPN comes out, each provider's account, and how many live
channels are playing. For more detail, each container has a log:

- `docker logs iptv-nginx` has the providers found at the last start, then one
  line per request.
- `docker logs iptv-relay` has a line each time a live MPEG-TS channel
  (shared, or fed from HLS) is opened or closed, how long the provider took to send the first video, and anything
  that went wrong with a channel.
- `docker logs iptv-vpn` is the VPN tunnel's own log.

Addresses are never logged by nginx or the relay, because they contain logins.

## With a playlist manager

A playlist manager is the tool some people use to trim, rename and reorder
what a provider offers. The proxy works with one, and where the proxy goes
depends on where the manager runs.

**A manager you run yourself, on your own network**

```
player  ->  playlist manager  ->  proxy  ->  provider
```

- In the manager, each provider source uses the proxy's address and port,
  with your easy login.
- Your players point at the manager, as they would without the proxy.
- The provider's real login lives only in the proxy's `.env`.
- The live format has to agree everywhere: the one chosen in your players,
  the one the manager hands out, and the provider's `NAME_LIVE_TV` line.

**A hosted manager, running on someone else's servers**

```
player  ->  proxy  ->  playlist manager  ->  provider
```

A hosted manager cannot reach a proxy inside your house, so the proxy goes in
front of it and treats the manager as the provider.

- In `.env`, add the manager as a provider: its server address as the URL,
  and your login for the manager as the username and password.
- Your players point at the proxy, with your easy login.
- When something is played, the manager answers "go to the provider's
  server". The proxy rewrites that answer as it does any redirect, so the
  video goes out through the VPN.
- The hosted manager itself holds your provider's real login and fetches the
  provider's lists from its own servers. That part does not go through your
  VPN; only what your players fetch does.

The first arrangement is the one this was built and tested with. The second
is the same mechanism but has not been run on this version. In particular,
live HLS through a hosted manager adds one more redirect on the way to the
video, and whether a channel's session is still shared across it has not been
tested.

In either arrangement, see the note on pictures under
[Trade-offs and things to know](#trade-offs-and-things-to-know).

## How it works

Most of what the proxy does is pass bytes along. Eight things go beyond that.
The first seven exist to keep the behavior the same as a direct connection,
and the last is there to help you. Engineers will find the details under
[How it works (technical)](#how-it-works-technical).

1. **Redirects stay inside the tunnel.** Providers answer most requests with
   "go to that other server". The proxy rewrites that answer so the player
   comes back to the proxy, which then fetches from the other server through
   the VPN. Without this the player would go to that server directly.
2. **Playlists and picture addresses are pointed back at the proxy.** An HLS
   playlist is a list of video pieces, and a provider's channel lists name a
   logo or poster for each entry. The proxy rewrites those addresses for the
   same reason.
3. **One session per live HLS channel.** Described [above](#why-this-exists).
   When the provider ends a session, the proxy opens a new one once and
   carries on.
4. **One easy login.** You choose a simple username and password for your
   players. The proxy swaps it for the provider's real login on the way out,
   so the real logins are typed in one place only.
5. **It refuses to be pointed at your own network.** A request cannot make the
   proxy fetch from the machine it runs on or from private addresses.
6. **Connections are kept open and reused.** A player connected directly keeps
   its connection to a server open and sends one request after another over
   it. The proxy does the same on both sides. A playlist answer leaves the
   player's connection open for the next refresh, and the proxy keeps its own
   connections to the provider's video servers open instead of setting one up
   for every playlist refresh and every video piece. Setting up an encrypted
   connection takes time, and one slow setup can use up a player's small live
   buffer and show as a stutter.
7. **One provider connection per live channel for MPEG-TS too.** This is the
   relay's main job. With `NAME_LIVE_TV=ts`, the first reader of a channel
   makes the relay open it at the provider, and everyone else on that channel
   is given the same video. With `NAME_LIVE_TV=hls`, a reader that asks for
   MPEG-TS (many recorders do) is given the channel's HLS video joined end to
   end, so it shares the session your players are on.
8. **A status page.** The relay draws a page that shows what is set up, what
   is wrong, where the VPN comes out and what is playing.

Your settings are read once, when the proxy starts. A short shell loop turns
each provider block in `.env` into its own listener on its own port.

No video server is named anywhere in the two files, because a provider can
move to other servers at any time. The proxy learns them instead:

- It notes the name of each server that video or a playlist really came from.
- A second short loop gives each name it has not seen a place on its list of
  servers to keep connections open to. It waits about 20 seconds for channel
  changes to settle, then has nginx re-read its settings once for all the
  names it collected.
- Until then, a server it has not seen works the plain way, with a connection
  set up for each request.
- The list is kept in a small Docker volume and loaded at every start, so a
  server is learned once. It is capped at 200 names, far more than a provider
  uses.
- Only video servers reached by name over HTTPS on the standard port are
  learned. Any other server, and one that only hands out pictures, works the
  plain way.

nginx runs as a single worker, so every stream shares the same set of open
connections.

If the relay is not running, TV still plays. nginx passes live MPEG-TS to the
provider itself, each reader on a connection of its own, and rewrites
playlists itself. Only the sharing in point 7 and the status page are missing
until the relay is back.

## What was tested, and how

The goal is parity with direct credentials, and no gap has been seen so far
on the real setup it was run on. Some of that was confirmed against real providers and some only
against simulated ones, so here is which is which.

**The relay is newer than the rest.** Its parts (MPEG-TS sharing, the
HLS-to-MPEG-TS bridge, playlist rewriting by the relay, the status page) were
run on one real setup on November 14, 2026, and against simulated providers.
On the real setup: three
players shared one MPEG-TS channel on one provider connection, joining and
leaving without disturbing each other; HLS played with playlists rewritten by
the relay; and an MPEG-TS reader was fed from a channel's HLS session while an
HLS player watched the same channel. That is hours of use, not weeks. The
lists below describe the proxy as it was before the relay was added.

**Confirmed against real providers:**

- Live TV over HLS and over MPEG-TS
- Movies, including skipping around in them
- Catch-up and time shift
- The guide and the provider's lists
- Changing channels
- Several readers on the same channel at once, over HLS
- Scheduled recordings
- A session ended by the provider being replaced without a failed request
- Restarting the proxy while a channel is playing
- Logos and posters loading through the proxy
- Several opens of one channel arriving at the same moment ending up on one
  session
- A playlist refresh reusing the player's open connection
- Video servers being learned from what was played, and connections to them
  being reused for playlists and video pieces
- The learned servers still being known after the proxy was restarted

Connection reuse was measured on a real provider by reading the proxy's log
while live channels played. About 99 percent of requests reused an open
connection, and the rest connected in a few hundredths of a second, with no
visible stutter over a long viewing session. Without reuse, every playlist
refresh and every video piece set up its own encrypted connection, and an
occasional one took about a second, which was enough to empty the player's
live buffer and show as a stutter.

**Confirmed against simulated providers only.** These are behaviors a provider
could have that the real ones used for testing did not, so a stand-in was
built for each. All of them pass, but none has been seen against a real
service:

- Playlists that name their pieces by bare file name (`chunk_123.ts`),
  including a further playlist in a sub-folder
- A provider that answers with the playlist itself instead of a redirect
- Redirects written in less common forms
- Playlist lines written as `//server/path` (another server, same http or
  https)
- Playlist lines written as `../folder/file` in a live channel's playlist,
  with a player that works the `..` out itself and with one that sends it as
  written. A few rarer forms are not handled there: `./../x`,
  `folder/../../x`, a bare `..`, and a line that climbs more than 32 folders
  above the server's top folder.
- Playlists labeled with an unusual content type
- Logins full of special characters (`@ # % & + = ? / " $ ; { } \`, an
  apostrophe, non-English letters)

**Not run:** the [platform notes](#platform-notes) other than plain Linux
with Docker Compose. They are written from each platform's documentation.

## Trade-offs and things to know

- **Xtream logins only.** A provider's ready-made M3U link (`get.php`) is
  refused, because the addresses inside it would skip the VPN. If an M3U link
  is all you were given, the Xtream login is inside it:
  `http://SERVER/get.php?username=USERNAME&password=PASSWORD&...`
- **Use the address you typed.** When a player logs in, the provider's reply
  includes the provider's own server address. The proxy passes that reply on
  unchanged. A player that connected to that address, instead of the proxy
  address you typed in, would reach the provider directly and skip the VPN.
- **Logos and posters go through the VPN too, with one exception.** A
  provider's channel and movie lists give each picture as a full web address,
  usually on some other site. The proxy points those addresses back at
  itself, so the player fetches the pictures through the tunnel. The
  exception is pictures named inside the guide file (`xmltv.php`): that file
  is passed on as is, so a player that shows guide artwork fetches it
  directly. They are only pictures; no video comes that way.
- **If a playlist manager sits between your player and the proxy,** the
  pictures are its business, not the proxy's. A playlist manager hands the
  player its own list, often with logos from its own picture library and with
  addresses it saved earlier. The proxy never sees those, so they are fetched
  directly. Only a player that talks to the proxy itself gets every picture
  through the VPN.
- **Pick one live format per provider and use it everywhere.** With
  `NAME_LIVE_TV=ts`, do not open a channel as HLS while it is playing or
  recording as MPEG-TS: most providers allow one kind at a time per channel,
  and the two will cut each other off.
- **If a provider offers both formats, HLS is the recommended setting.**
  MPEG-TS is one continuous stream, so how smoothly it plays depends on how
  evenly the provider sends video. The relay passes it on as it arrives and
  cannot smooth out a provider that sends in fits and starts.
- **A device that joins a channel is a little behind live.** With `ts`, a
  device that joins a channel someone else is already on starts about 10
  seconds behind. With `hls`, a recorder that asks for MPEG-TS gets the HLS
  video, which is a few seconds further behind live than the provider's own
  MPEG-TS.
- **The status page asks two outside parties questions.** To show where the
  VPN comes out it asks `ipinfo.io`, through the VPN. To show each account's
  state it asks the provider, through the proxy, at most once every 15
  seconds and only while someone has the page open. The page shows no logins, but like
  the proxy ports it is open to anyone on your home network.
- **If the relay stops, TV still plays.** nginx then passes live MPEG-TS to
  the provider itself, each reader on a connection of its own, and rewrites
  playlists itself. Sharing and the status page come back with the relay.
- **Your logins are in `.env` in plain text**, and the session memory
  (below) holds them too. Keep the folder private.
- **A video server is learned the first time it is used.** Until the proxy
  has seen a video server, each request to it sets up its own connection, as
  a plain proxy would. About 20 seconds after the channel changes settle, the
  proxy re-reads its settings once so it can keep connections to that server
  open, and that makes the player reconnect once. After that the server is
  remembered, across restarts too.
- **Kept-open connections cover video servers reached by name over HTTPS on
  the standard port.** That is how most providers serve video. A provider
  that serves it over plain HTTP, on another port, or from a bare number
  address still plays: each request to such a server sets up its own
  connection, as a plain proxy would.
- **A slow provider is still slow.** If a provider's server falls behind, the
  proxy cannot make up the difference. See
  [When something is wrong](#when-something-is-wrong) for how to tell that
  apart from a proxy problem.
- **Changing a VPN setting interrupts playback** for a few seconds while the
  tunnel reconnects. Changing a provider does not touch the VPN.

## What it stores, updating, removing

**What it stores.** Besides the two files you put there, two small Docker
volumes. Docker creates and manages them; there is no folder for you to make
or look after.

`<folder>_hls-memory` (for example `iptv-vpn_hls-memory` when the folder is
named `iptv-vpn`) is the session memory:

- For each live HLS channel being watched it holds one file of about 3 KB: the
  provider's "this channel is on that server" answer. The file includes the
  channel's address at the provider, **with your real login in it**.
- A channel's file is dropped one minute after the last request for it.
- It exists so that when the proxy container is recreated (you added a
  provider, or updated), channels that are playing carry on with the same
  session instead of being given a new one, which can show as buffering.

`<folder>_video-servers` (for example `iptv-vpn_video-servers`) is the list of
video servers the proxy has learned:

- It holds one small text file with one line per server, 200 at most. The
  lines contain server names only: no logins and no channel addresses.
- It exists so that a server seen once is already known at every later start,
  and the proxy does not have to re-read its settings while someone is
  watching.
- Nothing in it is needed to play. If it is removed, the proxy learns the
  servers again as they are used.

The relay stores nothing on disk. It keeps the newest video of each open
live channel in memory (about 15 seconds, between 12 MB and 64 MB a channel)
and is capped at 512 MB in all.

**Updating.**

- You changed `.env`: run `docker compose up -d`.
- You replaced `compose.yaml` with a newer version: run
  `docker compose up -d --force-recreate`. A plain `up -d` can leave the proxy
  running its old configuration.
- New versions of gluetun, nginx or Python: `docker compose pull`, then
  `docker compose up -d`. The images are not pinned to a version on purpose: a
  VPN client and a proxy should get their security fixes.
- Coming from the two-container version: replace `compose.yaml`, add a
  `NAME_LIVE_TV=hls` or `NAME_LIVE_TV=ts` line to every provider in `.env`
  (a provider without it is skipped), then run
  `docker compose up -d --force-recreate`.

**Removing.**

```
docker compose down -v
```

The `-v` matters: it removes both volumes too. A plain `down` leaves them
behind. Then delete the folder.

## Extras that need one more file

Two less common needs are met by a second file next to `compose.yaml`, named
`compose.override.yaml`. Docker Compose reads it automatically, so
`compose.yaml` still stays untouched.

**A VPN set up from an .ovpn file.** Put the file next to `compose.yaml`, name
it `my-vpn.ovpn`, and create `compose.override.yaml` with:

```yaml
services:
  vpn:
    volumes:
      - ./my-vpn.ovpn:/gluetun/custom.ovpn:ro
```

Then in `.env` use "Way 4" with `OPENVPN_CUSTOM_CONFIG=/gluetun/custom.ovpn`.

**A gluetun setting that is not in `.env.example`.** Add it the same way:

```yaml
services:
  vpn:
    environment:
      - SOME_GLUETUN_SETTING=value
```

## Platform notes

This is plain Docker Compose: two files in a folder and
`docker compose up -d`. That is how it was built and tested. The files are the
same everywhere; what differs is where they live and how you start them.

| Platform | Where to put the folder | How to start | Notes |
|---|---|---|---|
| Linux with Docker | anywhere, for example `/opt/iptv-vpn` | `docker compose up -d` | The reference setup. |
| Synology (Container Manager) | `/volume1/docker/iptv-vpn` | Project > Create, point it at the folder | Some models have no WireGuard in the kernel: set `WIREGUARD_IMPLEMENTATION=userspace`. |
| Proxmox | inside a VM running Docker | `docker compose up -d` | A VM is the simple route. In an LXC container you must pass `/dev/net/tun` through and allow nesting. |
| Windows (Docker Desktop with WSL 2) | a folder inside your WSL distribution | `docker compose up -d` | Use `WIREGUARD_IMPLEMENTATION=userspace`. Other devices reach it at the Windows machine's address; allow the ports in Windows Firewall. |
| Portainer | Stacks > Add stack, paste `compose.yaml` into the web editor | Deploy the stack | Under "Environment variables", use "Load variables from .env file" and pick your filled-in `.env`. |

## When something is wrong

- **Start with the status page.** Open
  `http://<the machine's address>:<any provider's port>/status` in a web
  browser. It says whether the proxy is working, names every provider that
  was left out and the line to fix, shows where the VPN comes out, and shows
  whether each provider accepted its login. If no provider could be started
  at all, the first port of `PROXY_PORTS` still answers so the page can tell
  you what is missing.
- **The status page says the relay is not answering.** Look at
  `docker logs iptv-relay`. TV keeps playing meanwhile, without the sharing.
- **A provider does not answer.** Read the `iptv-nginx` log from its last
  start. It names each provider it found and each one it skipped, with the
  reason. A provider with no `NAME_LIVE_TV` line is skipped.
- **Nothing answers.** Check that `iptv-vpn` is healthy. If the tunnel is
  down the proxy has no way out, by design.
- **A login with odd characters is refused.** Put it between single quotes in
  `.env`.
- **One channel buffers and the others are fine.** That is almost always the
  provider's server for that channel, not the proxy. In the log, each line
  shows how long the request took (the number after the size) and which
  server it came from (`uh=`). Slow lines that all name one server, while
  other channels are quick, point at that server. The same channel will be
  just as slow with the provider's login typed straight into a player.
- **Every channel buffers, on every device.** Suspect the VPN server before
  the provider. A VPN server you pick by host name can turn out to be a
  far-away entry point even when its exit shows a nearby city. A slow or
  lossy VPN server causes buffering on every device at once. Check the
  round-trip time from the machine that runs the stack to the VPN server it
  connected to (with `ping`, for example), and look for lost packets. If the
  time is high or packets go missing, choose another server.
- **Two devices cut each other off on one channel.** Check that the
  provider's `NAME_LIVE_TV` matches the live format your players really use.
  With `ts`, a channel opened as HLS while it is playing or recording as
  MPEG-TS still ends one of the two: that is the provider's rule.
- **Reading the nginx log.** `k=` is the kind of request. `s=` shows whether a
  channel's session was reused (`HIT`), opened (`MISS`) or replaced
  (`BYPASS`). `uh=` is the server the answer came from. `uct=` is how long
  it took to connect to that server, in seconds; `0.000` means an open
  connection was reused.
- **Reading the relay log.** Every line starts with `RELAY:`. It says when a
  channel was opened and closed and how many are open, how long the provider
  took to send the first video, when a reader joined or left and why, when a
  provider sent nothing for more than a few seconds and then carried on, and
  when a provider ended a channel that was being watched and the relay asked
  for it again. A channel watched as plain HLS leaves no line here: nginx
  serves it, and its requests are in the nginx log.

## How it works (technical)

This part is for engineers. Everything in it is taken from `compose.yaml`,
which holds the nginx configuration, the start script and the relay's source,
each with comments. Where this page and the file disagree, the file is right.

### Three containers, one network namespace

| Service | Container | Image | Job |
|---|---|---|---|
| `vpn` | `iptv-vpn` | `qmcgaw/gluetun` | Holds the tunnel and publishes the `PROXY_PORTS` range. |
| `nginx` | `iptv-nginx` | `nginx:alpine` | All HTTP proxying. One listener per provider. |
| `relay` | `iptv-relay` | `python:3-alpine` | Long-lived live streams, playlist rewriting, the status page. |

- `nginx` and `relay` both run with `network_mode: "service:vpn"`. They have no
  network of their own: they share the gluetun container's interfaces,
  firewall and loopback. Anything they fetch leaves through the tunnel, and if
  the tunnel is down they have no other way out.
- Because the loopback is shared, the pieces talk to each other on
  `127.0.0.1` with no Docker network between them. nginx reaches the relay on
  `127.0.0.1:8197`, the relay reaches nginx on a provider's own port, and
  nginx keeps its HLS session memory on `127.0.0.1:8195`. gluetun's control
  server is held to `127.0.0.1:8000`. None of these is published.
- The only published ports are the `PROXY_PORTS` range, on the `vpn` service.
  A provider port may not be 53, 8000, 8195, 8197 or 9999, which are in use
  inside the namespace.
- `nginx` and `relay` each wait for `vpn` to be healthy and are restarted with
  it. Each one's health check also looks for a global-scope address. If the
  tunnel container was recreated underneath it, the container is left in a
  dead namespace; the check then waits 12 seconds and stops the main process,
  so Docker starts the container again inside the tunnel's new network.
- `vpn` is given a fixed list of gluetun settings by name. `nginx` and `relay`
  read the whole settings file (`.env`, or `stack.env` under Portainer), and
  each one's start command strips the environment down before the real
  program runs: nginx starts with `PATH` and `TZ` only, the relay with `PATH`
  and a list of provider addresses.

### What nginx does

**At start.** The entrypoint script finds every `NAME` that has `NAME_URL` in
the environment and checks it: all five lines present, a port inside
`PROXY_PORTS` that is not reserved and not used twice, an address of the form
`http(s)://name[:port]`, and `NAME_LIVE_TV` equal to `hls` or `ts`. For each
good one it writes a `server` block with the listen port, the provider
address, the percent-encoded login and the live format, and includes the
shared locations. For each bad one it prints `PROVIDER NAME SKIPPED:` with the
reason. The same findings are written to a small JSON file that nginx serves
to loopback callers only, which is where the status page gets them. If no
provider could be started, one listener on the first port of the range still
serves `/status`, and the container reports itself unhealthy.

**Proxying.** HTTP/1.1 to the upstream, response buffering off so bytes are
passed on as they arrive, a 10 second connect timeout and a 60 second limit
on the gap between bytes (not on stream length). Upstream certificates are
not verified, which is nginx's default: many players do not check them
either, and some providers' servers carry expired or self-signed ones.

**Redirects and addresses.** A `Location` naming another server is rewritten
to `/__cdn/<scheme>/<host>/...` on the proxy, so the player comes back and the
fetch goes through the tunnel. A `/__cdn/` target may not be `localhost`, a
loopback, private, link-local or carrier-grade NAT address, a bracketed IPv6
literal, a host containing `@` or `%`, or a numeric address written with
leading zeros. The same list is applied to the redirect a live HLS channel is
sent to. Picture addresses in `player_api.php` answers are rewritten to the
same `/__cdn/` form. `get.php` is refused with 403.

**The login swap.** A request that carries `PLAYER_USER` and `PLAYER_PASSWORD`
in the path or the query string is sent upstream with the provider login of
the port it arrived on. Nothing else in the address is changed. Any other
login is passed on untouched and refused by the provider.

**Session memory for live HLS.** An internal server on `127.0.0.1:8195` is an
nginx proxy cache that stores only redirect answers, keyed by provider and
channel path. Opening a channel asks that cache where the provider sent it,
then hands the player a folder of its own, `/__ch/<channel address>/`. Every
later refresh of the playlist, and every piece named by bare file name, is
resolved against the remembered redirect, so all readers of a channel stay on
one provider session. Opens that arrive together wait for the first one (cache
lock, up to 20 seconds). An entry is dropped one minute after its last use.
When the remembered session answers with an error, the channel is opened
again once, bypassing the cache, and the new redirect is remembered. The
cache directory is a Docker volume so that playing channels keep their
session when the nginx container is recreated.

**Connection reuse and learned video servers.** nginx keeps upstream
connections open only to servers it has an `upstream` block for, and no
server is named in the file. So a log line is written to a small file for
every request answered with 200 or 206 by a server reached by name on port
443 with a video or playlist content type. A loop in the entrypoint reads
that file every 5 seconds. When it sees a new name it waits 20 seconds for
channel changes to settle, writes one `upstream` block per new server (8 idle
connections, each let go after 20 seconds without use, `max_fails=0` so a server is never set
aside), tests the configuration and reloads nginx once. The list is saved to
a second Docker volume, read back at start through a strict pattern, and
capped at 200 entries.

**One worker.** The entrypoint sets `worker_processes 1`. The image default is
one worker per CPU thread, and each worker has its own pool of upstream
connections, so a stream that bounced between workers kept opening new ones.
A home proxy carries a handful of streams, and one worker reuses the same
connections for all of them.

**Logging.** One line per request with the port, status, sizes, timings, user
agent, kind of request, session-memory result, upstream host and connect
time. The request address is never logged, because Xtream addresses contain
the login. nginx's error log is limited to critical messages for the same
reason.

**What it hands to the relay.** Three kinds of request are proxied to
`127.0.0.1:8197`: a live MPEG-TS request made with the easy login (to `/ts`
or `/hls`, according to that provider's `NAME_LIVE_TV`), a `GET` for a
playlist under `/__ch/`, `/__cdn/` or `/__cdu/` (to `/pl`), and `/status`.
nginx tells the relay what it needs in `X-Relay-*` request headers. On every
request handed to the relay, nginx sets or blanks all of those headers
itself, and the `X-Hls-*` ones used by the session memory are blanked on
every request that does not use it. So the relay and the session memory
never see a value sent by a player.

### What the relay does

The relay is one asyncio program. It listens on `127.0.0.1:8197`, so nginx is
its only caller. It has four endpoints that matter, plus `/healthz` for the
container's health check and `/stat`, a one-line count of open channels and
readers.

| Endpoint | Job |
|---|---|
| `/ts` | Share one provider MPEG-TS stream between every reader of a channel. |
| `/hls` | Give a reader that asks for MPEG-TS the channel's HLS video, joined end to end. |
| `/pl` | Point the addresses inside an HLS playlist back at the proxy. |
| `/status` | Draw the status page. |

For `/ts` and `/hls`, nginx sends the provider's address, the channel's
address at the provider (with the provider login already swapped in), the
address as the player sent it, and the port. The relay answers 403 unless the
provider is one of the addresses it was started with. A `HEAD` request is
answered with 200 on the spot, because asking the provider would open a
connection. One channel has one key whichever form of its address a player
uses (`/live/user/pass/1.ts`, `/user/pass/1.ts`, `/user/pass/1`). The relay
serves at most 100 readers at once, all channels together.

### MPEG-TS sharing (`/ts`)

- **One upstream per channel.** The first reader of a channel makes the relay
  open it at the provider, with that player's own request headers. Every
  later reader of the same channel is attached to the same upstream. The
  provider sees one connection however many readers there are, and one
  reader leaving does not disturb the rest.
- **A ring of recent video.** For each open channel the relay keeps the
  newest video in memory: about 15 seconds of it, never less than 12 MB and
  never more than 64 MB.
- **Where a joiner starts.** The reader that opens a channel gets the stream
  from its first byte. A reader that joins later is handed the newest 10
  seconds of video at once, so it starts about 10 seconds behind live and has
  something in hand when the provider pauses, as a player on its own
  connection has from the provider's opening burst. The amount is counted in
  seconds, not bytes: the relay measures each channel's real rate over the
  last minute and over the last 15 seconds, takes the larger, and multiplies.
  It never hands over more than the last 20 seconds of arrivals. In a channel's first 8 seconds there is no rate
  yet, and a joiner gets everything kept so far. The joiner's first byte is
  placed on a program table (PAT) packet if one is near, otherwise on the
  first whole packet.
- **Slow readers are cut off.** Each reader has its own queue. A reader that
  has been more than 16 MB behind its channel for 15 seconds, or is ever 64
  MB behind, is disconnected, so that what is held for it stays bounded. What
  a joiner was handed at the start is not counted against it, until it has
  been sent on. This is also
  how long a paused player is waited for.
- **Last reader out.** When the last reader leaves, the provider's connection
  is dropped at once. The exception is a channel in its first 5 seconds,
  where a tuning player often hangs up and asks again within a moment: the
  channel lingers for 0.4 seconds unless that player has just opened another
  channel.
- **Changing channel.** A player hangs up on the old channel and asks for the
  new one at almost the same moment, and the two can arrive in either order.
  Before a new channel is opened, the relay drops that provider's channels
  that nobody is reading, gives a hang-up that is just behind the request a
  moment (30 ms) to arrive, and leaves the provider 0.1 seconds after the
  drop to notice it. If the player has hung up by then, nothing is opened.
  The aim is that a one-connection account never sees two connections during
  a channel change.
- **A stream the provider ends.** If the provider ends a stream, or it sends
  nothing for 60 seconds, while readers are still attached, the relay asks
  for the channel again after 0.1 seconds and carries on, so the players do
  not each have to notice and reconnect. It does this only for a stream that
  had run for at least 30 seconds, and not when another channel of the same
  provider was opened in the last 15 seconds (on a one-connection account
  that is why it ended). Asked again, the provider gets 5 seconds to answer
  and 10 to send video; after that the readers are let go, as they would be
  on a connection of their own. A channel that
  goes silent twice in a row, each time within 90 seconds of being asked for,
  is treated as dead: the readers are let go and the provider is not asked
  again. The new stream is joined on at a clean packet, and a
  packet the old stream left half-sent is filled out first so the packets
  stay aligned.
- **Refusals.** If the provider refuses the channel, its status code and the
  body of its answer are passed to the player. A reader waits up to 60
  seconds for a channel's first video.

### The HLS-to-MPEG-TS bridge (`/hls`)

With `NAME_LIVE_TV=hls`, a live `.ts` request does not go to the provider as
MPEG-TS. The relay builds the address of the same channel as HLS on this
proxy, `/__ch/<channel>.m3u8/__index.m3u8` on the provider's port, and reads
it from nginx on loopback, exactly as a player would. That puts the reader on
the session memory described above, so it shares the one provider session
with every HLS player on the channel.

- It reads the playlist, follows a list of qualities to the highest
  `BANDWIDTH` on offer, and starts near the newest video (the last three
  pieces).
- It fetches each new piece once, in order, and sends the pieces end to end
  as one MPEG-TS stream. It asks for the playlist again at the playlist's
  target duration, or half of it when nothing was new.
- Readers of the same bridged channel share one such feed, with the same ring
  and joiner rules as `/ts`.
- A piece that cannot be fetched after two tries is skipped and noted in the
  log; three in a row end the stream. A playlist that cannot be had is asked
  for again (4 tries at the start, 9 once playing).
- It only joins playlists made of whole, unencrypted MPEG-TS files. A
  playlist with `EXT-X-MAP`, `EXT-X-BYTERANGE` or an `EXT-X-KEY` other than
  `METHOD=NONE` cannot be joined end to end and is not bridged.
- Every address it fetches must be this proxy's own. A playlist line that
  leads anywhere else is not fetched.
- The stream ends when the playlist carries `EXT-X-ENDLIST` or stops moving
  for 30 seconds (or six target durations, if that is longer).

The bridged stream is the HLS one, so it runs a few seconds further behind
live than the provider's own MPEG-TS would.

### Playlist rewriting (`/pl`)

nginx has always rewritten playlists with text rules (`sub_filter`). Those
rules are still there, and the relay now does the job first because it reads
a playlist line by line, the way a player does, and so copes with forms that
text matching would miss.

1. A player's `GET` for a playlist is handed to the relay with the original
   address and port.
2. The relay asks nginx for that same address from the source address
   `127.0.0.2`. For that one caller nginx fetches the playlist from the
   provider as usual (same session, same kept connections) but its text rules
   put back exactly what they found, so the playlist arrives as the provider
   wrote it. nginx adds three response headers that say where this
   playlist's addresses lead: the proxy path for the server it came from, the
   same for scheme-relative addresses, and the proxy path for the playlist's
   own folder one level up.
3. The relay rewrites every address line and every `URI="..."` inside a tag:
   full `http://` and `https://` addresses, `//server/path`, `/path` and
   `../path`. An address with none of those beginnings sits next to the
   playlist and is left as it is. Bytes outside addresses come out as they
   went in. The player's request headers and the server's response headers
   are passed through, apart from per-connection ones, range headers,
   `Server` and `Date` (nginx adds its own), and the proxy's own notes.
4. nginx strips those three headers from anything an upstream sends, so only
   nginx can set them and a server cannot use them to point a player
   somewhere else.

**The fallback.** If the relay does not answer (it is not running, or it
steps aside because the body is not a playlist, is over 1 MB or has more than
20,000 lines), nginx gets a failed upstream and serves the request itself: it
re-requests the same address from `127.0.0.3`, a caller for which the text
rules work as they always did. A playlist that arrives compressed, which
should not happen because nginx asks for plain text, cannot be read by the
relay. It steps aside in the same way and nginx serves the playlist itself,
as it came.

### The status page (`/status`)

nginx proxies `/status` on any provider port to the relay, with the port and
the easy login in headers. The relay then:

- reads the start-up notes from nginx (what was found, what was skipped and
  why)
- asks `https://ipinfo.io/json` for the public address, through the tunnel,
  with certificate checking on, at most once every 5 minutes (once a minute
  while the lookup is failing)
- asks each provider for its account details through nginx, with the easy
  login, so the request takes the same path a player's would. This happens when
  someone views the page, at most once every 15 seconds, and the page waits up
  to 6 seconds for the answer so that what it shows is current. Each answer is
  labeled with its age ("Checked just now"), and the page reloads itself every
  30 seconds. If the provider is slow, the last good answer is shown with its
  age, for up to 15 minutes. After a failure the provider is asked less and
  less often (up to 15 minutes apart).
- counts live channels: HLS channels whose playlist was asked for in the last
  30 seconds (by a player, or by the relay for a recorder), MPEG-TS channels and their readers, and MPEG-TS readers
  being fed from HLS. Movies, series and catch-up are not counted.

The page returns 200 when at least one provider is set up and there is no
settings problem, and 503 otherwise. It shows no logins.

### When the relay is not running

| Request | What nginx does instead |
|---|---|
| Live MPEG-TS | Passes it to the provider directly, each reader on a connection of its own. |
| A playlist | Rewrites it with its own text rules. |
| `/status` | Answers 503 with one line that says to look at the relay's log. |

Everything else (the API, the guide, movies, series, catch-up, HLS pieces,
the session memory, the login swap) never goes through the relay.

### How the relay is fenced in

As written in `compose.yaml`:

- **Network.** It shares the tunnel's namespace and listens on loopback only.
  It connects to the providers named in `.env` and the servers they redirect
  to, to nginx in this stack, and to `ipinfo.io` for the status page. Before
  it follows a redirect it looks the name up itself, refuses a private,
  loopback, link-local or carrier-grade NAT address, and connects to the very
  address it checked. Its fetches from nginx are never allowed to be
  redirected to another host.
- **What it is given from `.env`.** The start command works out which
  providers are set up, using the main checks nginx uses (the name, the
  address form, the lines being present, `hls` or `ts`), and starts Python
  with a cleared environment holding only `PATH` and the list of those
  providers' addresses. No login, password or VPN key is in the program's
  environment. It does see a provider's login when nginx hands it a live
  channel's address, and the easy login when the status page is drawn; it
  keeps neither on disk.
- **User.** It runs as `65534:65534` (nobody), with no added capabilities and
  no volumes.
- **Memory.** `mem_limit: 512m`, a ceiling in case a provider misbehaves.
- **Health check.** Every 30 seconds: it must still have an address in the
  tunnel's namespace, and `/healthz` on `127.0.0.1:8197` must answer.
- **Logs.** Its log lines never contain an address or a login.
- **Dependencies.** None. The script uses only the Python standard library
  and refuses to start on a Python older than 3.12.

### Why Python, and why not inside nginx

The design rule is that nginx keeps doing what it is good at, which is HTTP
proxying, and nothing else is added unless plain nginx settings cannot do the
job. Four things could not be done with settings, or not well enough:
fanning one upstream stream out to several readers, joining HLS pieces into
one stream, reading a playlist line by line, and drawing a status page.

The first attempt, which was never published, did the stream work inside
nginx with its JavaScript module (njs). Measured on real streams, it either
grew in memory for as long as video flowed through it, or, when that was
worked around, corrupted the stream. Neither is acceptable in the video
path. So the long-lived stream handling moved out to a small asyncio program:
one process, standard library only, in the official Python image, with its
source inline in the compose file like the nginx settings. nginx still
terminates every player connection and still does all proxying to providers
for everything that is not a shared live stream.

### Limits, stated plainly

- **MPEG-TS smoothness depends on how evenly the provider sends video.** The
  relay passes video on as it arrives and does not pace it. A joiner has
  about 10 seconds in hand. If a provider pauses for longer than a player has
  buffered, that player stalls, as it would on a direct connection.
- **HLS is the recommended setting where a provider offers it.** With HLS the
  player fetches finished pieces and keeps its own buffer, nginx's session
  memory survives a recreate of the nginx container, and a recorder that
  wants MPEG-TS is still served through the bridge.
- **With `ts`, HLS and MPEG-TS do not mix on one channel.** Opening a channel
  as HLS while it is being read as MPEG-TS still ends one of the two. That is
  the provider's rule, and turning MPEG-TS into HLS is beyond this proxy.
- **The bridge needs plain HLS.** Encrypted or byte-range playlists, and ones
  with an `EXT-X-MAP` line, are not bridged.
- **Recreating the relay container ends the live streams that run through
  it.** That happens when `.env` changes or the stack is updated.
- **Certificates of providers are not verified**, by nginx or by the relay,
  for the reason given above.
- **The relay's parts are newer than the rest.** See
  [What was tested, and how](#what-was-tested-and-how).

## Support

There is none. This was made for personal use and is shared as is, free, in
case it is useful to someone else. Issues and pull requests are turned off,
and questions will not be answered. Everything needed to use it is on this
page. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE). gluetun, nginx and Python are separate projects with their own
licenses. This repository contains configuration for them and one small
Python program, and does not distribute any of the three.
