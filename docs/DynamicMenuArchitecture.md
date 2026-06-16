# Dynamic Menu Architecture

This architecture provides a scalable, static-first design for a menu system that is both professional in performance and flexible for administrative updates. It adheres to the constraint of a static Astro frontend and avoids database dependencies by leveraging Git and R2 as the sources of truth.

## 1. Core Philosophy
- **Authoritative Source:** The menu structure is defined in a `menu.json` file stored in the Git repository.
- **Performance:** Menu data is bundled into the Astro build at compile-time, ensuring zero-latency loading and perfect SEO on the Cloudflare edge.
- **Images:** All menu background images are stored in R2 (following the existing catalog architecture), keeping the Git repository lightweight.

## 2. Data Structure
The `menu.json` file uses a generic, recursive-ready schema:

[
  {
    "id": "item-id",
    "label": "Display Name",
    "url": "/path/",
    "order": 1,
    "isVisible": true,
    "image": {
      "url": "https://api.example.com/api/menu-image/filename.jpg",
      "alt": "Image description"
    }
  }
]

## 3. The Lifecycle
This design employs a "Git-as-CMS" pattern, bridging the gap between dynamic admin needs and static site delivery.

1. **Rendering:**
    - The `DesktopMenu.svelte` island imports `menu.json` directly.
    - Svelte iterates over the array using a generic `{#each}` loop to construct the UI dynamically.
2. **Updating:**
    - The Admin UI provides an interface to manage labels, links, and image uploads.
    - When an image is uploaded, it is routed via the Go backend to the R2 bucket.
    - Upon saving the menu configuration, the Go backend generates a JSON payload and sends a `repository_dispatch` event to GitHub.
3. **Deployment:**
    - A GitHub Action receives the event, writes the new `menu.json` to the source tree, and commits/pushes the changes.
    - Cloudflare Pages detects the git push and triggers an automated rebuild, deploying the updated menu to the global CDN.

## 4. Technical Components
- **`site/public/data/menu.json`**: The static file that drives the UI.
- **`DesktopMenu.svelte`**: A data-driven component that eliminates hard-coded menu items.
- **Go Backend**: Orchestrates the communication between the R2 storage, the Admin UI, and the GitHub API.
- **GitHub Actions**: Acts as the "build engine," ensuring the menu state is versioned in Git and published via Cloudflare.

## 5. Benefits
- **No Database:** Eliminates the need for schema migrations and database hosting while maintaining structured data.
- **Speed:** The menu is baked into the static HTML, eliminating runtime API calls and layout shifts.
- **Resilience:** The menu state is fully versioned in Git; if an admin makes a mistake, reverting to a previous menu state is a standard git operation.
- **Consistency:** By reusing the existing R2 and API patterns established for the catalog, the codebase remains modular and predictable.
