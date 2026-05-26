# Astro Starter Kit: Basics

```sh
npm create astro@latest -- --template basics
```

> 🧑‍🚀 **Seasoned astronaut?** Delete this file. Have fun!

## 🚀 Project Structure

Inside of your Astro project, you'll see the following folders and files:

```text
/
├── public/
│   └── favicon.svg
├── src
│   ├── assets
│   │   └── astro.svg
│   ├── components
│   │   └── Welcome.astro
│   ├── layouts
│   │   └── Layout.astro
│   └── pages
│       └── index.astro
└── package.json
```

To learn more about the folder structure of an Astro project, refer to [our guide on project structure](https://docs.astro.build/en/basics/project-structure/).

## 🧞 Commands

All commands are run from the root of the project, from a terminal:

| Command                   | Action                                           |
| :------------------------ | :----------------------------------------------- |
| `npm install`             | Installs dependencies                            |
| `npm run dev`             | Starts local dev server at `localhost:4321`      |
| `npm run build`           | Build your production site to `./dist/`          |
| `npm run preview`         | Preview your build locally, before deploying     |
| `npm run astro ...`       | Run CLI commands like `astro add`, `astro check` |
| `npm run astro -- --help` | Get help using the Astro CLI                     |

## 👀 Want to learn more?

Feel free to check [our documentation](https://docs.astro.build) or jump into our [Discord server](https://astro.build/chat).


# Labriideas Publisher
A high-performance system for distributing large music catalogs with minimal bandwidth and instant load times.

## System Overview
The system uses a "Content-Delivery Proxy" architecture:
- **Storage:** Cloudflare R2 (Immutable object storage).
- **Versioning:** HTTP ETags (MD5 hashes) provide atomic version control.
- **Delivery:** Gzip-compressed data streams served via Go.
- **Client:** Svelte-powered frontend with local-first compressed caching.

## Getting Started
1. Configure your `.env` with `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, etc.
2. Run the server: `go run main.go`
3. Run the frontend: `npm run dev`

## Documentation
- [Architecture Overview](./docs/ARCHITECTURE.md)
- [API Reference](./docs/API.md)
- [Crawler Reference](./docs/CRAWLER.md)
