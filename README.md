<h1 align="center">
    ──「 ғɪʟᴇ sᴛᴏʀᴇ  - ɢᴏ ᴇᴅɪᴛɪᴏɴ 」──
</h1>

<p align="center">
  <img src="https://raw.githubusercontent.com/gotd/td/main/.github/logo.png" alt="FileStore Go logo" width="150" />
</p>

<p align="center">
<a href="https://github.com/aquib4040/filestore/stargazers"><img src="https://img.shields.io/github/stars/aquib4040/filestore?color=yellow&logo=github&logoColor=yellow&style=for-the-badge" alt="Stars" /></a>
<a href="https://github.com/aquib4040/filestore/network/members"><img src="https://img.shields.io/github/forks/aquib4040/filestore?color=blue&logo=github&logoColor=blue&style=for-the-badge" alt="Forks" /></a>
<a href="https://github.com/aquib4040/filestore/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License" /></a>
<a href="https://go.dev/"><img src="https://img.shields.io/badge/Written%20in-Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go" /></a>
<a href="https://t.me/canon_bots"><img src="https://img.shields.io/badge/Telegram-Channel-blue?style=for-the-badge&logo=telegram&logoColor=white" alt="Telegram Channel" /></a>
</p>

---

## ⚡ Why Go is Better than Python for Telegram Bots

| Metric | Python (Pyrogram/Telethon) | Go (gotd/td) |
| :--- | :--- | :--- |
| **Idle Memory Usage** | ~120MB - 180MB RAM | **~5MB - 10MB RAM** 🟢 |
| **Concurrency Model** | OS Threads / AsyncIO overhead | **Goroutines (2KB memory each)** 🟢 |
| **Execution Speed** | Interpreted (Slow) | **Compiled Native Binary (Fast)** 🟢 |
| **Image Size (Docker)** | 250MB - 400MB+ | **~25MB (Multi-stage Alpine)** 🟢 |
| **Multi-Tenant Limit** | ~4-6 bots on a $4/mo VPS | **50 - 100+ bots on a $4/mo VPS** 🟢 |

*   **Massive Memory Savings**: Each Go clone bot consumes only **~5MB to 10MB RAM**, allowing you to host **dozens of bots** simultaneously on a single $4 VPS.
*   **True Concurrency**: Utilizes Go **Goroutines** which require only 2KB of stack memory, delivering high concurrency speeds without the GIL bottleneck of Python.
*   **Zero Interpreters**: Compiles to a single lightweight binary with no virtualenv setup needed.

---

## 🚀 One-Click Cloud Deploys

Deploy your high-performance FileStore bot instantly to cloud environments:

[![Deploy to Render](https://img.shields.io/badge/Deploy%20to-Render-black?style=for-the-badge&logo=render)](https://render.com/deploy?repo=https://github.com/aquib4040/filestore)
[![Deploy to Heroku](https://img.shields.io/badge/Deploy%20to-Heroku-purple?style=for-the-badge&logo=heroku)](https://www.heroku.com/deploy/?template=https://github.com/aquib4040/filestore)
[![Deploy to Koyeb](https://img.shields.io/badge/Deploy%20to-Koyeb-blue?style=for-the-badge&logo=koyeb)](https://app.koyeb.com/deploy?repository=github.com/aquib4040/filestore&branch=main&name=go-filestore)
[![Deploy to Zeabur](https://img.shields.io/badge/Deploy%20to-Zeabur-darkgreen?style=for-the-badge&logo=zeabur)](https://zeabur.com)

*(For Zeabur, simply log in, select **Create Project**, and link your repository: `https://github.com/aquib4040/filestore`)*

---

## 🌟 Key Features

*   **👥 Multi-Bot Cloning**: Allows standard users to deploy and configure their own cloned bots with custom configurations using the `/clone` command.
*   **🔒 Secure Symmetric Token Encryption**: Bot tokens are encrypted in MongoDB using AES-GCM-256 derived from a custom security passphrase, protecting them from raw db leaks.
*   **🔗 Scoped Auto Commands API**: Automatically registers Telegram menu command shortcuts based on user scopes (Admins see administrative options, users see default help menus).
*   **📨 Live Status Broadcasting with Progress Tracker**: Admins can broadcast messages to all users in the background with real-time status reporting, interactive progress bars (e.g. `[████░░░░░░] 40%`), and automatic `tgerr.AsFloodWait` sleep-retries.
*   **💬 Copy-Broadcasting Support**: Supports forwarding complete message structures (including media files, formatted text, and inline keyboards) by replying `/broadcast` to a target message.
*   **⏱️ Personalized Downloader Preferences (`/mysettings`)**: Users can customize their download experiences by toggling their own personal auto-delete timers (1m, 5m, 15m, 30m) and content protection settings.
*   **📡 Force Subscription (FSub)**: Restricts access to files until users join configured channels. Supports **Join Request Approval** channels.
*   **🔋 Anti-Sleep Worker (HTTP Self-Ping)**: Automatically pings the bot's health check server every 3 minutes using the `FQDN` configuration, keeping free-tier containers (Render, Zeabur) awake 24/7.

---

## 🕹️ Command Reference

### Main Bot Commands

| Command | Allowed Users | Description |
| :--- | :--- | :--- |
| `/start` | All Users | Greets the user or retrieves shared files |
| `/mysettings` | All Users | Opens the personal downloader preferences panel |
| `/settings` | Admin/Owner | Opens the main bot settings panel |
| `/clone` | Admin/Owner | Prompts user to start a new cloned bot instance |
| `/deletecloned`| Admin/Owner | Prompts user to delete a running cloned bot instance |
| `/broadcast` | Admin/Owner | Broadcasts text or replied message (with inline buttons) to all users |
| `/stats` | Admin/Owner | Displays database size, active clones, and system metrics |
| `/genlink` | Admin/Owner | Generates a single file download link |
| `/batch` | Admin/Owner | Generates a range file batch download link |

### Cloned Bot Commands

| Command | Allowed Users | Description |
| :--- | :--- | :--- |
| `/start` | All Users | Greets the user or retrieves shared files |
| `/mysettings` | All Users | Opens the personal downloader preferences panel |
| `/premium` | All Users | Displays pricing coordinates and payment UPI ID |
| `/settings` | Bot Creator | Opens the clone dashboard (Shorteners, custom text, UPI ID, Plans, FSub/DB Channels) |
| `/broadcast` | Bot Creator | Broadcasts text or replied message to this clone's specific user base |

---

## ⚙️ Configuration Variables

Add these parameters inside your local `.env` configuration file:

```env
# --- TELEGRAM API CONFIGURATION ---
API_ID=1234567               # Get from my.telegram.org
API_HASH="your_api_hash"     # Get from my.telegram.org
BOT_TOKEN="your_bot_token"   # Get from @BotFather

# --- MONGO DATABASE CONFIGURATION ---
MONGO_URI="your_mongodb_uri" # MongoDB Cluster connection string
DB_NAME="filestore"

# --- OWNER CONFIGURATION ---
OWNER_ID=123456789           # Numeric Telegram User ID of primary owner

# --- OPTIONAL CONFIGURATIONS ---
PORT=8080                    # Port for HTTP health checks
AUTO_DEL=300                 # Delay (seconds) to auto-delete shared files (0 to disable)
DISABLE_BTN=true             # Block sharing buttons on media forwards
PROTECT=true                 # Block forwarding/saving files from the bot
CLONE_LIMIT=3                # Maximum cloned bots allowed per User ID
CLONE_ALLOW=true             # Globally enable/disable cloning feature (true/false)

# --- SECURITY (ENCRYPTION) ---
TOKEN_ENCRYPTION_KEY="pass"  # Symmetric key used to encrypt bot tokens in DB

# --- KEEP-AWAKE WORKER ---
FQDN="mybot.render.com"      # App domain to activate the self-ping worker
```

---

## 🖥️ VPS Deployment

### Method 1: Using Docker-Compose (Recommended)

1.  **Clone the project**:
    ```bash
    git clone https://github.com/aquib4040/filestore.git
    cd filestore
    ```
2.  **Create your env file**:
    ```bash
    cp .env.example .env
    nano .env
    ```
3.  **Boot up services**:
    ```bash
    docker-compose up --build -d
    ```

### Method 2: Running Native Binary

Ensure you have **Go 1.21+** installed:

1.  **Download modules**:
    ```bash
    go mod download
    ```
2.  **Compile bot**:
    ```bash
    go build -o filestore-bot ./cmd/bot
    ```
3.  **Run service**:
    ```bash
    ./filestore-bot
    ```

---

## 📢 Channels & Support

Join our official channel for bot releases, templates, and support queries:

*   **Telegram Channel**: [@canon_bots](https://t.me/canon_bots)
