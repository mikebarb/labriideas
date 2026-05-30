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
**Back End:** Systems.
- [Architecture Overview](./docs/ARCHITECTURE.md)
- [API Reference](./docs/API.md)
- [Crawler Reference](./docs/CRAWLER.md)
- [API Design Reference](./docs/API_DESIGN.md)


**Front End:** Web Pages.
- [Front End Web Design](.docs/FrontEndWebDesign.md)

**Developers:** Environment Setup.
- [Development Setup](./docs/DEVELOPMENT.md)