# Portfind (पोर्टफ़ाइंड)

> **जानें कि आप क्या बंद कर रहे हैं (Know what you're killing).** प्रोजेक्ट संदर्भ (Project context), रिस्क टियर्स (Risk tiers) और शून्य आकस्मिक डाउनटाइम के साथ Windows और Linux के लिए एक सुरक्षित, आधुनिक पोर्ट और प्रोसेस मैनेजर।

<div align="center">

[![Language: English](https://img.shields.io/badge/Language-English-blue?style=for-the-badge)](README.md)
[![Language: Hindi](https://img.shields.io/badge/भाषा-हिन्दी-orange?style=for-the-badge)](#)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=for-the-badge)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux-0078D6?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/rpratap2111/portfind/releases)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://go.dev/)

### विषय टैग (Topic Tags)
[![Topic: Port Management](https://img.shields.io/badge/विषय-पोर्ट_प्रबंधन_(Port_Management)-0052CC?style=for-the-badge&logo=target&logoColor=white)](#)
[![DevTools](https://img.shields.io/badge/इकोसिस्टम-डेवलपर_टूल्स-FF6F00?style=for-the-badge&logo=visualstudiocode&logoColor=white)](#)

### तकनीकी स्टैक (Tech Stack)
[![Go / Golang](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![SQLite](https://img.shields.io/badge/SQLite-modernc.org%2Fsqlite-003B57?style=for-the-badge&logo=sqlite&logoColor=white)](https://modernc.org/sqlite)
[![Windows API](https://img.shields.io/badge/Windows_API-GetExtendedTcpTable-0078D6?style=for-the-badge&logo=windows&logoColor=white)](#)
[![Linux pidfd](https://img.shields.io/badge/Linux-pidfd_%26_procfs-FCC624?style=for-the-badge&logo=linux&logoColor=black)](#)
[![PowerShell & Bash](https://img.shields.io/badge/स्क्रिप्ट्स-PowerShell_%26_Bash-5391FE?style=for-the-badge&logo=powershell&logoColor=white)](#)

</div>

---

## संक्षिप्त विवरण (Brief Summary)

**portfind** Windows और Linux पर किसी भी नेटवर्क पोर्ट पर चल रहे प्रोसेस (प्रक्रिया) को खोजता है और उसे सुरक्षित रूप से बंद (kill) करने की सुविधा देता है। पारंपरिक टूल्स के विपरीत, प्रोसेस को बंद करने से पहले यह आपको दिखाता है कि वह प्रोसेस **किस प्रोजेक्ट** से संबंधित है, उसकी पूरी कमांड लाइन क्या है, वह कितनी देर से चल रहा है, और उसे बंद करना कितना जोखिम भरा (Risk Level) हो सकता है।

यह मुख्य रूप से तीन रूपों में उपलब्ध है:
- **टर्मिनल UI** (`portfind`): Windows और Linux दोनों पर कीबोर्ड-चालित इंटरैक्टिव इंटरफ़ेस।
- **सिस्टम ट्रे आइकन** (`portfind-tray`): Windows के टास्कबार नोटिफिकेशन एरिया में चलने वाला हल्का मेनू।
- **JSON CLI मोड** (`portfind --json`): स्क्रिप्ट्स और ऑटोमेशन के लिए तेज़ और स्ट्रक्चर्ड आउटपुट।

### मुख्य विशेषताएँ (Core Highlights):
- **प्रोजेक्ट की पहचान:** प्रोसेस की डायरेक्टरी को स्कैन करके `package.json`, `Cargo.toml`, `go.mod` या Git रिपॉजिटरी से प्रोजेक्ट का असली नाम पता करता है।
- **रिस्क श्रेणियाँ (LOW, MEDIUM, HIGH):** Dev रनटाइम्स (Node, Python, Vite) को आप तुरंत बंद कर सकते हैं, जबकि डेटाबेस (PostgreSQL, MySQL, Redis) या सक्रिय SSH सेशन्स को गलती से बंद होने से बचाने के लिए नाम टाइप करवाता है।
- **रीसाइकिल्ड PID से सुरक्षा:** प्रोसेस को बंद करने से पहले उसके हैंडल (`pidfd` / Windows handle) को सुरक्षित करता है, ताकि कोई नया शुरू हुआ प्रोसेस गलती से बंद न हो जाए।
- **पोर्ट-फाइट डिटेक्शन:** यदि कोई ऑटो-रीस्टार्ट टूल (जैसे `nodemon`) पोर्ट पर बार-बार प्रोसेस को रीस्टार्ट कर रहा है, तो यह चेतावनी देकर अनचाहे लूप से बचाता है।
- **इतिहास (History Log):** SQLite डेटाबेस के माध्यम से बंद किए गए सभी प्रोसेसेज़ का पूरा रिकॉर्ड रखता है।

---

```
╭──────────────────────────────────────────────────────────────────────────────────────────────╮
│ Search: 30█                          ⚡ :3000 killed 3× in 15m (node · my-app): something keeps… │
│                                                                                              │
│ ╭──────────────────────────────────────────────────────────────────────────────────────────╮ │
│ │ PORT   PID     PROJECT        PROCESS   AGE     RISK    COMMAND                          │ │
│ │ 3000   12240   my-app         node      2h15m   LOW     "C:\nodejs\node.exe" server.js   │ │
│ │ 3306   7840    -              mysqld    ?       HIGH                                     │ │
│ │ 5173   20412   portfolio      node      45s     LOW     node ...\vite\bin\vite.js        │ │
│ ╰──────────────────────────────────────────────────────────────────────────────────────────╯ │
│                                                                                              │
│ 27 ports · 3 matching · refreshed 14:03:12                                                   │
│ ↑/↓ nav  type to search  enter kill  tab history  ctrl+r refresh  ctrl+w warnings  esc quit  │
╰──────────────────────────────────────────────────────────────────────────────────────────────╯
```

---

## इन्स्टॉलेशन (Install)

### Windows

इसके लिए 64-बिट Windows 10 या 11 (x64 या ARM64) आवश्यक है। एडमिन (Admin) अधिकारों की आवश्यकता नहीं है।

#### एक कमांड से इन्स्टॉल करें (अनुशंसित)

PowerShell में निम्न कमांड चलाएँ:

```powershell
irm https://raw.githubusercontent.com/rpratap2111/portfind/main/install.ps1 | iex
```

फिर चलाएँ:

```powershell
portfind
```

इन्स्टॉलर आपके CPU के अनुसार नवीनतम रिलीज डाउनलोड करता है, **SHA-256 चेकसम की पुष्टि करता है**, `portfind.exe` और `portfind-tray.exe` को `%LOCALAPPDATA%\portfind\bin` में रखता है और उसे आपके यूजर PATH में जोड़ देता है।

<details>
<summary>इन्स्टॉलर के विकल्प (Installer options)</summary>

कमांड चलाने से पहले इन्हें सेट कर सकते हैं:

```powershell
$env:PORTFIND_VERSION = "v0.1.0"               # नवीनतम के बजाय कोई विशिष्ट संस्करण
$env:PORTFIND_INSTALL_DIR = "D:\tools\portfind" # डिफ़ॉल्ट के बजाय कोई अन्य डायरेक्टरी
$env:PORTFIND_NO_MODIFY_PATH = "1"              # PATH को न बदलें
```

</details>

#### मैन्युअल डाउनलोड

1. [Latest Release](https://github.com/rpratap2111/portfind/releases/latest) से `portfind_windows_amd64.zip` (या ARM के लिए `portfind_windows_arm64.zip`) डाउनलोड करें।
2. इसे कहीं भी एक्सट्रैक्ट करें। टर्मिनल UI के लिए `portfind.exe` या ट्रे आइकन के लिए `portfind-tray.exe` चलाएँ (दोनों फाइलें एक ही फोल्डर में रखें)।
3. (वैकल्पिक) उस फोल्डर को अपने PATH में जोड़ें ताकि आप किसी भी टर्मिनल से `portfind` रन कर सकें।

> **Windows SmartScreen सूचना:** बाइनरीज़ कोड-साइन नहीं हैं, इसलिए ब्राउज़र से डाउनलोड करने पर Windows "Windows protected your PC" दिखा सकता है। **More info → Run anyway** पर क्लिक करें। PowerShell इन्स्टॉलर के प्रयोग से आमतौर पर यह प्रॉम्प्ट नहीं आता।

#### Go के साथ

यदि आपके पास Go 1.26 या उससे नया संस्करण है:

```powershell
go install github.com/rpratap2111/portfind/cmd/portfind@latest
```

#### अनइन्स्टॉल (Uninstall)

पहले portfind को बंद करें (ट्रे आइकन पर राइट-क्लिक करके **Quit** करें), फिर चलाएँ:

```powershell
irm https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.ps1 | iex
```

यह निष्पादन योग्य फाइलों और PATH प्रविष्टि को हटा देता है। आपका इतिहास सुरक्षित रहता है। यदि इतिहास भी हटाना चाहते हैं, तो पहले यह चलाएँ:

```powershell
$env:PORTFIND_PURGE_HISTORY = "1"
```

अन्य माध्यम से इन्स्टॉल किया था?
- **`go install`:** `%USERPROFILE%\go\bin\portfind.exe` को डिलीट करें।
- **मैन्युअल डाउनलोड:** एक्सट्रैक्ट किए गए फोल्डर को डिलीट करें और PATH से हटाएँ।

---

### Linux

64-बिट Linux (x86_64 या arm64) की आवश्यकता है; कोई भी डिस्ट्रो। रूट (root) की आवश्यकता नहीं है।

```sh
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | sh
```

इन्स्टॉलर आपके CPU के लिए नवीनतम रिलीज डाउनलोड करता है, **SHA-256 चेकसम की पुष्टि करता है**, और `portfind` को `~/.local/bin` में रखकर `~/.bashrc` (या `~/.zshrc`) में PATH जोड़ देता है। एक नया टर्मिनल खोलें और `portfind` चलाएँ। यह `sudo` के अंतर्गत भी काम करता है, जिससे यह अन्य उपयोगकर्ताओं के प्रोसेस को भी देख और रोक सकता है।

<details>
<summary>इन्स्टॉलर विकल्प और अनइन्स्टॉल</summary>

```sh
# विशिष्ट रिलीज या अन्य डायरेक्टरी:
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_VERSION=v1.1.0 sh
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_INSTALL_DIR=~/bin sh

# शेल प्रोफाइल को न बदलें:
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/install.sh | PORTFIND_NO_MODIFY_PATH=1 sh

# अनइन्स्टॉल:
curl -fsSL https://raw.githubusercontent.com/rpratap2111/portfind/main/uninstall.sh | sh
```

या [Latest Release](https://github.com/rpratap2111/portfind/releases/latest) से `portfind_linux_amd64.tar.gz` डाउनलोड करके एक्सट्रैक्ट करें।

</details>

---

## portfind ही क्यों? (Why portfind?)

पारंपरिक पोर्ट किलर जैसे [pik](https://github.com/jacek-kurlit/pik), pview, PortSlayer सिर्फ एक सवाल पूछते हैं: *पोर्ट 3000 पर कौन सा प्रोसेस है?* वे केवल PID और प्रोसेस नाम दिखाते हैं। लेकिन `:3000` पर `node` आपका भूला हुआ प्रोजेक्ट भी हो सकता है, या आपके साथी की महत्वपूर्ण सर्विस भी! portfind एक अलग सवाल पूछता है: **"मैं वास्तव में क्या बंद करने जा रहा हूँ?"**

| सुविधा (Feature) | सामान्य पोर्ट किलर | **portfind** |
|---|:---:|:---:|
| पोर्ट, PID, प्रोसेस का नाम | ✓ | **✓** |
| **प्रोजेक्ट का नाम** (`package.json`, `Cargo.toml`, `go.mod` या Git रिपो से) | ✗ | **✓** |
| **पूरी कमांड लाइन** (सिर्फ `node.exe` नहीं, बल्कि `node server.js`) | ✗ | **✓** |
| **अप-टाइम (कितनी देर से चल रहा है)** | ✗ | **✓** |
| **रिस्क टियर (जोखिम का स्तर)** जो पुष्टि की आवश्यकता तय करता है | ✗ | **✓** |
| बंद करने से ठीक पहले PID की पुनः जाँच (गलत प्रोसेस बंद न हो) | ✗ | **✓** |
| **पोर्ट-फाइट डिटेक्शन:** बार-बार रीस्टार्ट होने वाले प्रोसेस की पहचान | ✗ | **✓** |
| **किल हिस्ट्री ट्रैकिंग** (SQLite में सुरक्षित इतिहास) | ✗ | **✓** |
| **Windows नोटिफिकेशन ट्रे** से 1-क्लिक सुरक्षित किल | ✗ | **✓** |

---

## उपयोग (Usage)

`portfind` चलाएँ। यह सभी सक्रिय लिसनिंग TCP पोर्ट्स को दिखाता है, हर 2 सेकंड में रीफ्रेश होता है और आपके सर्च व सिलेक्शन को बनाए रखता है।

| की (Key) | कार्य (Action) |
|---|---|
| *कुछ भी टाइप करें* | पोर्ट, प्रोसेस या प्रोजेक्ट से फ़िल्टर करें (फ़ज़ी सर्च: `dwa` से `demo-web-app` मिलेगा; `node 30` दोनों को खोजेगा) |
| `↑` `↓` | सिलेक्शन को ऊपर या नीचे ले जाएँ |
| `Enter` | चुने गए प्रोसेस को बंद (kill) करें (पुष्टि माँगेगा) |
| `Tab` | पोर्ट छोड़ने वाले पुराने प्रोसेस का इतिहास देखें |
| `Ctrl+R` | तुरंत रीफ्रेश करें |
| `Ctrl+W` | चेतावनियाँ देखें (जैसे वे प्रोसेस जिन्हें OS ने पढ़ने की अनुमति नहीं दी) |
| `Esc` | डायलॉग या व्यू से बाहर निकलें; पोर्ट सूची से बाहर आने पर बंद होगा |
| `Ctrl+C` | प्रोग्राम बंद करें (Quit) |

---

### `--json` के साथ स्क्रिप्टिंग

`portfind --json` सभी लिसनिंग पोर्ट्स को JSON प्रारूप में प्रिंट करके बाहर निकल जाता है (बिना किसी UI के):

```json
{
  "ports": [
    {
      "port": 8899,
      "pid": 17012,
      "process": "python",
      "project": "git-only-repo",
      "age_seconds": 2,
      "risk": "LOW",
      "command": "\"C:\\...\\python.exe\" -m http.server 8899",
      "parent_pid": 5920,
      "parent_process": "pwsh"
    }
  ],
  "warnings": ["port 135 pid 1984 (svchost): OpenProcess: Access is denied."]
}
```

#### PowerShell उदाहरण:

```powershell
# पोर्ट 3000 पर क्या चल रहा है?
(portfind --json | ConvertFrom-Json).ports | Where-Object port -eq 3000

# आपके प्रोजेक्ट्स द्वारा घेरे गए सभी पोर्ट्स देखें
(portfind --json | ConvertFrom-Json).ports | Where-Object project | Format-Table port, process, project
```

#### jq उदाहरण:

```sh
portfind --json | jq '.ports[] | select(.risk == "LOW") | {port, process, project}'
```

`portfind --version` संस्करण की जानकारी देता है।

---

### रिस्क श्रेणियाँ (Risk Tiers)

| श्रेणी (Tier) | क्या शामिल है | बंद करने का तरीका |
|---|---|---|
| **LOW** | डेवलपमेंट रनटाइम्स: `node`, `python`, `ruby`, `java`, `dlv`, `go run` द्वारा शुरू किए गए प्रोसेस | `[y/N]` प्रॉम्प्ट में केवल `y` दबाएँ |
| **MEDIUM** | अज्ञात प्रोसेस और सामान्य सिस्टम सर्विसेज़ | सुरक्षा के लिए प्रोसेस का नाम टाइप करें |
| **HIGH** | डेटाबेस (`postgres`, `mysql`, `mongod`, `redis-server`, `sqlservr`) और SSH द्वारा शुरू प्रोसेस | सुरक्षा के लिए प्रोसेस का नाम टाइप करें |

बंद करने से पहले, portfind प्रोसेस को पिन (Windows पर हैंडल, Linux पर pidfd) करता है, ताकि उसका PID किसी अन्य नए प्रोसेस द्वारा दोबारा उपयोग न हो सके।

- **Windows:** प्रोसेस को टर्मिनेट किया जाता है। कोर सिस्टम प्रोसेस (जैसे `lsass`, `csrss`, `wininit`, `services`, `svchost`) को कभी बंद नहीं किया जाता क्योंकि इससे सिस्टम क्रैश हो सकता है।
- **Linux:** प्रोसेस को पहले `SIGTERM` भेजा जाता है ताकि वह सुरक्षित बंद हो सके, और 5 सेकंड बाद भी चलने पर `SIGKILL` भेजा जाता है। `systemd`, `init`, `sshd` और `systemd-resolved` को कभी बंद नहीं किया जाता।

---

### ट्रे आइकन (Windows Tray Icon)

`Win` दबाएँ, **portfind** खोजें और खोलें (या `portfind-tray` चलाएँ)। नोटिफिकेशन एरिया में एक आइकन दिखाई देगा। आइकन पर क्लिक करने से उन पोर्ट्स की सूची खुलेगी जिन्हें आप सामान्यतः बंद करना चाहते हैं (डेवलपमेंट सर्वर सबसे ऊपर):

```
portfind · 40 listening ports
──────────────────────────────────────────────
python — :8899 (git-only-repo)        LOW    ▸ ┌──────────────────────────────────┐
mystery-daemon — :9300 (fixtures)     MEDIUM ▸ │ PID 12528 · running 4s · LOW risk │
…                                              │ Project: git-only-repo            │
20 system or elevated ports not shown          │ "…\python.exe" -m http.server 8899│
…and 8 more (Open Terminal UI to see all)      │ Kill python                       │
──────────────────────────────────────────────  └──────────────────────────────────┘
Open Terminal UI
✓ Start with Windows
Quit
```

- LOW प्रोसेस पर **Kill** क्लिक करते ही वह तुरंत बंद हो जाता है और नोटिफिकेशन से पुष्टि होती है।
- MEDIUM या HIGH प्रोसेस पर **Kill…** पहले एक पुष्टिकरण डायलॉग दिखाता है जिसमें प्रोजेक्ट, PID और कमांड की जानकारी होती है।
- **Start with Windows** से आप सिस्टम शुरू होने पर ट्रे आइकन को स्वतः शुरू कर सकते हैं।

---

### पोर्ट फाइट और इतिहास (Port Fights and History)

जब भी कोई प्रोसेस पोर्ट छोड़ता है, portfind उसे रिकॉर्ड करता है। यदि आपने किसी पोर्ट को **15 मिनट में 3 बार** बंद किया है, तो सर्च बॉक्स के पास चेतावनी (`⚡`) दिखाई देगी—आमतौर पर `nodemon` या कोई सुपरवाइजर उसे बार-बार फिर से शुरू कर रहा होता है। इतिहास देखने के लिए `Tab` दबाएँ।

इतिहास SQLite में सुरक्षित रहता है:
- **Windows:** `%LOCALAPPDATA%\portfind\history.db`
- **Linux:** `~/.cache/portfind/history.db`

---

## सीमाएँ (Limitations)

- **Windows और Linux:** macOS अभी समर्थित नहीं है। ट्रे आइकन केवल Windows के लिए है; Linux पर टर्मिनल UI का उपयोग करें।
- **अन्य उपयोगकर्ताओं व एडमिन प्रोसेस:** Windows पर बिना एडमिन अधिकारों के आप सिस्टम प्रोसेस की पूरी जानकारी नहीं देख सकते। पूर्ण नियंत्रण के लिए portfind को Administrator के रूप में चलाएँ। Linux पर `sudo portfind` चलाएँ।
- **प्रोजेक्ट डिटेक्शन:** प्रोसेस की वर्तमान वर्किंग डायरेक्टरी के आधार पर प्रोजेक्ट का पता लगाता है।
- **केवल TCP:** यह केवल TCP लिसनिंग पोर्ट्स दिखाता है (UDP नहीं)।

---

## सोर्स कोड से बिल्ड करें (Build From Source)

```powershell
git clone https://github.com/rpratap2111/portfind
cd portfind
go build ./cmd/portfind
.\portfind.exe
go build -ldflags -H=windowsgui ./cmd/portfind-tray  # GUI बिल्ड: कंसोल विंडो नहीं खुलेगी
.\portfind-tray.exe
```

प्योर Go (बिना cgo): SQLite ड्राइवर `modernc.org/sqlite` है, और Windows को सीधे Win32 APIs (`GetExtendedTcpTable`, `NtQueryInformationProcess`, आदि) के माध्यम से बिना `netstat` पार्स किए सीधे क्वेरी किया जाता है।

```powershell
go test ./...
```

### रिलीज़िंग (Releasing)

वर्जन टैग पुश करें। GitHub Actions टेस्ट चलाएगा और [GoReleaser](https://goreleaser.com) ज़िप फाइलें और `checksums.txt` बनाकर रिलीज प्रकाशित करेगा:

```powershell
git tag v0.1.0
git push origin v0.1.0
```

---

## लाइसेंस (License)

[MIT](LICENSE) लाइसेंस के अंतर्गत वितरित।
