# Development Environment Setup

This guide provides step-by-step instructions for establishing a clean development environment for the **Labriideas Publisher** system on Windows.

## 1. Prerequisites (Installation)
*   **Git for Windows:** Download from [git-scm.com](https://git-scm.com/). Ensure you select "Checkout as-is, commit Unix-style line endings".
*   **Go (v1.22+):** Install using the Windows MSI from [go.dev/dl/](https://go.dev/dl/). Ensure `go` is in your system PATH.
*   **Node.js (LTS):** Install from [nodejs.org](https://nodejs.org/). 
    *   **CRITICAL:** During the installation wizard, ensure the checkbox **"Automatically install the necessary tools"** (Python/Build Tools) is checked. This ensures npm packages with native C++ dependencies build successfully.
*   **VS Code:** Install with the following extensions:
    *   `Go` (by Go Team at Google)
    *   `Astro` (Language support)
    *   `Svelte for VS Code`

## 2. Workspace Initialization
1.  **Clone:** Clone your repository into your local projects directory.
2.  **Workspace File:** Always open the project using the **`.code-workspace`** file found in the project root. This ensures the multi-root directory structure is mapped correctly.
3.  **Environment Variables:** Create a `.env` file in the **Publisher Directory**. This file is ignored by Git and must exist locally. See the provided template **.env.example.txt** which contains:
    ```text
    R2_ACCOUNT_ID=your_account_id
    R2_ACCESS_KEY_ID=your_access_key
    R2_SECRET_ACCESS_KEY=your_secret_key
    R2_BUCKET_NAME=your_bucket_name
    ```
4.  **Extensions:** Upon opening the workspace, VS Code will prompt to install recommended extensions. Click **"Install All"** to ensure standardized linting and syntax highlighting for Go and Svelte.
Ensure you have installed:
```text
    Astro (Language Support for Astro)
    Go (by Go Team at Google)
    Svelte for VS Code
    Markdown Preview Mermaid Support (by Matt Bierner).
```

## 3. Toolchain & Dependencies
### Backend (Go)
1.  Open a terminal in the root directory.
2.  Initialize/Update modules: `go mod tidy`.
3.  *Note:* If prompted by VS Code to install "Missing Go Tools" (e.g., `gopls`, `dlv`), click **"Install All"** to ensure backend IDE features function correctly.

### Frontend (Astro/Svelte)
1.  Navigate to your frontend directory (e.g., `cd site`).
2.  Install dependencies: `npm install`.
3.  Install library to calculate MD5 in the browser: `npm install spark-md5`.
3.  Install library manage drag and drop in the browser: `npm install svelte-dnd-action`.
4.  To allow coding the PWA capability: `npm install workbox-window`.

## 4. Operational Commands
Open two terminal instances within VS Code:
*   **Backend:** 'cd publisher' then `go run main.go`
*   **Frontend:** `cd web` then `npm run dev`

## 5. Troubleshooting & Tips
*   **Port Conflicts:** If `go run main.go` fails with "address already in use", use `netstat -ano | findstr :8080` in PowerShell to find and kill the blocking process.
*   **Permissions:** If `npm install` throws EPERM (Permission Denied) errors, restart your terminal as **Administrator**.
*   **Terminal Management:** If your terminal windows move into separate native OS windows, press `Ctrl+Shift+P` and type **"Terminal: Move Terminal into Panel"** to dock them back into the main editor.
*   **File Nesting/Explorer:** If your directory tree looks "flat," check VS Code settings for **"Explorer: Compact Folders"** and toggle it off to view the full hierarchy.
