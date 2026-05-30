# Frontend Web Page Design: L'Abri Ideas Library

## 1. Executive Summary
The L'Abri Ideas Library architecture is designed to transition from a restrictive, platform-dependent environment to a lightweight, high-performance, and maintainable static-site structure. The design emphasizes semantic HTML, search engine optimization (SEO), and an academic aesthetic.

## 2. Technology Stack
* **Framework:** [Astro](https://astro.build/) for routing, metadata handling, and static site generation.
* **UI Components:** [Svelte](https://svelte.dev/) for state-heavy interactivity (Mega-menus, Search, and Tab navigation).
* **Styling Engine:** [Tailwind CSS v4](https://tailwindcss.com/) for a responsive, utility-first UI.
* **Typography:** System-safe serif/sans-serif stack, optimized for fast rendering and readability.

## 3. Significant Design Decisions
* **Centralized Data Management:** Navigation and category structures are defined in `src/data/menu.js`. This declarative approach ensures that menu updates propagate automatically across the site, preventing link rot.
* **Layering and Composability:** Graphical elements, such as the brushstroke overlay, use absolute-positioned SVG vectors. This allows for programmatic scaling and performance efficiency without the weight of large image assets.
* **Component-Driven Architecture:** UI elements are isolated into distinct, reusable Svelte components. Logic (e.g., hover states or dropdown timers) is encapsulated, keeping individual files maintainable.
* **Responsiveness:** The navigation system uses a `flex-wrap` strategy rather than `hidden` breakpoints. This ensures the menu wraps gracefully on smaller screens, keeping all content accessible at all times.

## 4. Architecture Diagram
The following diagram illustrates the composition of the frontend components.

```mermaid
graph TD
    Layout[LabriLayout.astro] --> Nav[Navbar.svelte]
    Nav --> MegaMenu[MegaMenu.svelte]
    MegaMenu --> Data((menu.js))
    Layout --> Slot(Page Content Slot)
    Slot --> Hero[Hero.svelte]
    Slot --> Content[Main Section]
    Layout --> Footer[Footer.svelte]
