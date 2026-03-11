# 📧 labelr - Auto Classify and Label Gmail Emails

[![Download labelr](https://img.shields.io/badge/Download-Here-blue?style=for-the-badge)](https://github.com/SHellysss/labelr/releases)

---

## ❓ What is labelr?

labelr is a simple tool that helps you organize your Gmail inbox. It works on your computer and uses AI to sort emails into categories automatically. It runs without sending your data online, keeping your information private. You only need to run a command on your computer, and labelr will start classifying your emails for you.

This tool is for anyone who wants to save time sorting through emails. You don’t need technical skills or software knowledge to use it.


## 💻 System Requirements

Before installing labelr, make sure your computer meets these basic needs:

- Runs Windows 10 or later  
- At least 4 GB of free RAM  
- At least 100 MB of free disk space  
- Internet connection to authorize your Gmail account  
- Command Prompt access (comes with Windows)  

labelr uses AI and runs locally. It does not require a heavy setup or programming tools.


## 🚀 Getting Started

Follow these steps to get labelr running on your Windows computer.

### 1. Download labelr

Click the button below to visit the releases page where you can download the latest version of labelr for Windows. The packages are ready to run and do not require installation tools.

[![Download labelr](https://img.shields.io/badge/Download-Here-brightgreen?style=for-the-badge)](https://github.com/SHellysss/labelr/releases)

On the page, look for a file named like:

`labelr_windows_amd64.exe`  

or similar. This is the program you’ll run.

### 2. Save the file

When you click the file, choose a folder to save it to. We suggest your Desktop or your Downloads folder, so you can find it easily.

Do not move or rename the file after downloading it.

### 3. Open Command Prompt

labelr runs in a command window. To open it:

- Press `Win + R` on your keyboard  
- Type `cmd`  
- Press Enter  

A black window with text will appear. This is your Command Prompt.

### 4. Run labelr

In the Command Prompt window type the full file path where you saved labelr. For example:

```
C:\Users\YourName\Desktop\labelr_windows_amd64.exe
```

Then press Enter.

If the file is in your current folder, you can type:

```
.\labelr_windows_amd64.exe
```

If you see a message asking for permission to access Gmail, allow it. labelr will guide you through signing into your Gmail account. This is needed so it can safely access your emails.

### 5. Let labelr work

Once connected, labelr will start scanning your Gmail inbox and assign labels to your emails. This process may take a few minutes depending on how many emails you have.

You do not need to keep the Command Prompt open all the time. You can close it anytime, but labelr will pause until you run it again.

---

## 📂 How labelr Works

labelr uses AI to classify your Gmail emails based on their content. It reads your emails on your computer only and applies labels to your Gmail account. These labels help you find and sort emails faster.

The tool runs using the command line. It does not have a graphical user interface. This keeps it simple and fast.

labelr supports the following features:

- Automatic reading and labeling of new emails  
- Supports Gmail’s native label system  
- Labels based on email topics like work, personal, spam, updates  
- Local storage of processing data, no cloud upload  
- Works offline after initial setup  

The AI model is lightweight and works with your Gmail directly. It improves your productivity by cutting time spent on organizing emails.


## 🔧 Settings and Configuration

You can tweak labelr’s behavior by creating a simple settings file.

- Create a file called `config.json` in the same folder as the `.exe` file.  
- Use basic settings like label categories and frequency of runs.  

Example `config.json` content:

```json
{
  "check_interval_minutes": 30,
  "labels": ["Work", "Personal", "Spam", "Updates"]
}
```

This file tells labelr to check for new emails every 30 minutes and use these four labels.

If no settings file is present, labelr will work with default categories and check once daily.

---

## 🛠 Fixing Common Issues

### Command not recognized

Make sure you typed the exact file name and path in Command Prompt. Use quotes if there are spaces in folder names:

```
"C:\Users\Your Name\Desktop\labelr_windows_amd64.exe"
```

### Permission denied or Cannot access Gmail

labelr needs you to sign into your Gmail account. Follow the on-screen instructions. If you have two-factor authentication, be ready to confirm login.

### Emails not labeled

Check your Gmail to make sure you gave labelr access. Also, verify you are connected to the internet during setup.


## ⬇️ Download & Installation Summary

1. Visit this page to download labelr:  
   https://github.com/SHellysss/labelr/releases

2. Download the Windows executable file.

3. Save it to an easy location.

4. Open Command Prompt (`Win + R` → type `cmd` → Enter).

5. Run the file by typing its full path or `.\filename.exe` if in the same folder.

6. Follow prompts to sign into Gmail and authorize.

7. Let labelr label your emails automatically.

---

## 📚 Additional Information

labelr was built using Go programming language and uses open AI frameworks that operate locally. This design keeps your workflow private and does not send emails to external servers.

You can stop and restart the program anytime using Command Prompt.

For advanced users, labelr supports command-line options to customize behavior. Type:

```
.\labelr_windows_amd64.exe --help
```

to see available options.

---

## 🗂 About labelr

- Classifies Gmail emails with AI  
- Runs locally on Windows machines  
- Has no GUI, uses commands  
- Supports Gmail label management  
- Built using Golang and open AI APIs  
- Designed for privacy and productivity  

---

## 📞 Getting Support

If you experience issues, you can open a request on the GitHub Issues page here:  
https://github.com/SHellysss/labelr/issues

Provide details about your Windows version, steps you took, and the problem you faced. This will help the developers respond efficiently.