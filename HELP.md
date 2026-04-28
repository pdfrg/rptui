## Installing the Required mpv Player
Your Radio Paradise TUI app needs mpv (a lightweight media player) to play the audio files.

Don't worry if you're not technical!
mpv is very easy to install. Just copy and paste the commands for your operating system.
If you get stuck, feel free to open an issue and we'll help.

Installing it is simple on every major platform.

### Windows (Recommended: Scoop)

1. Open PowerShell (search for it in the Start menu).
2. Copy and paste these commands one by one, pressing Enter after each:

Configure PowerShell
```PowerShell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

Install Scoop (Windows package manager):
```PowerShell
irm get.scoop.sh | iex
```

Add extras repository to Scoop
```PowerShell
scoop bucket add extras
```

Install mpv
```PowerShell
scoop install extras/mpv
```
3. Verify it worked by typing:

```PowerShell
mpv --version
```

That's it! Scoop is the easiest option for most people and keeps everything tidy.

### macOS (Using Homebrew)

1. If you don't have Homebrew yet, install it first (copy-paste this one long command into Terminal):

```Bash
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
```
2. Install mpv:

```Bash
brew install mpv
```
3. Verify:

```Bash
mpv --version
```

### Linux

Ubuntu / Debian / Pop!_OS / Linux Mint (and derivatives):

```Bash
sudo apt install mpv
```

Fedora:

```Bash
sudo dnf install mpv
```
Arch Linux / Manjaro / EndeavourOS / Omarchy:

```Bash
sudo pacman -S mpv
```

Other distros: Search for "install mpv" + your distro name, or use Flatpak as a universal fallback:

```Bash
flatpak install flathub io.mpv.Mpv
```
After installing on any platform, just run rptui — it should automatically find and work with mpv now.

## Installing a NerdFont (needed to display symbols/icons)

rptui uses special icons from Nerd Fonts. These do not come standard with Windows, macOS, or most Linux distributions, so you need to install one.

Recommended fonts for terminal/TUI apps (pick one):
- JetBrainsMono Nerd Font
- FiraCode Nerd Font

### Windows

- Go to: https://www.nerdfonts.com/font-downloads
- Download one of the recommended fonts above (click the download icon next to it — it downloads a .zip file).
- Extract (unzip) the downloaded file. (R click > Extract)
- Select all the .ttf files inside the folder.
- Right-click the selected files → Install for all users (or just Install on newer Windows).
- Restart your terminal (Windows Terminal or PowerShell).

### macOS

If you already have Homebrew installed (from the mpv instructions), this is the easiest way:

```Bash
brew tap homebrew/cask-fonts
brew install --cask font-jetbrainsmono-nerd-font
```
Replace font-jetbrainsmono-nerd-font with font-firacode-nerd-font if you prefer.

Then restart your terminal (iTerm2, Terminal app, Warp, etc.).

### Linux

Ubuntu / Debian / Pop!_OS / Mint:

```Bash
mkdir -p ~/.local/share/fonts
cd ~/.local/share/fonts
wget https://github.com/ryanoasis/nerd-fonts/releases/download/v3.2.1/JetBrainsMono.zip
unzip JetBrainsMono.zip
rm JetBrainsMono.zip
fc-cache -fv
```

Fedora:

```Bash
mkdir -p ~/.local/share/fonts
cd ~/.local/share/fonts
wget https://github.com/ryanoasis/nerd-fonts/releases/download/v3.2.1/JetBrainsMono.zip
unzip JetBrainsMono.zip
rm JetBrainsMono.zip
fc-cache -fv
```

Arch and derivatives:

```Bash
sudo pacman -S nerd-fonts-jetbrains-mono
```
After installing, restart your terminal.

### If symbols are still not visible: configure your terminal to use the Nerd Font

On Linux, generally it is not necessary to change your terminal font.
The terminal will look for the symbols in any installed font and automatically use them.

However, in some cases, you may need to tell your terminal app to use the Nerd Font specifically.

Windows Terminal: Settings → Profiles → Defaults → Appearance → Font face → select “JetBrainsMono NF” (or similar)

iTerm2 (macOS): Preferences → Profiles → Text → Font → choose the Nerd Font

Most Linux terminals (GNOME Terminal, Konsole, Alacritty, etc.): Look for Font or Appearance settings and select the Nerd Font.
Some terminals may require editing their config file manually.
