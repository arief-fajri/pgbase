import { defineConfig } from "vitepress";

export default defineConfig({
    title: "PGBase Docs",
    description: "PostgreSQL-powered backend as a service (PocketBase fork) — architecture, flows, features.",
    // Project pages are served from a subpath: https://arief-fajri.github.io/pgbase/
    // Local dev therefore serves at http://localhost:5174/pgbase/
    base: "/pgbase/",
    srcDir: ".",
    outDir: ".vitepress/dist",
    cacheDir: ".vitepress/cache",
    themeConfig: {
        nav: [
            { text: "Home", link: "/index" },
            { text: "Fork deltas", link: "/fork-deltas" },
            { text: "Collections & API rules", link: "/collections-and-api-rules" },
            { text: "Production", link: "/single-host-production" },
            { text: "Backup & Observability", link: "/backup-restore-observability" },
            { text: "Contributing", link: "/contributing-releasing" },
        ],
        sidebar: [
            {
                text: "Start",
                items: [
                    { text: "Overview", link: "/index" },
                    { text: "Fork deltas", link: "/fork-deltas" },
                    { text: "Collections & API rules", link: "/collections-and-api-rules" },
                    { text: "Roadmap", link: "/roadmap" },
                ],
            },
            {
                text: "Architecture",
                items: [{ text: "Backend layers", link: "/architecture/backend-layers" }],
            },
            {
                text: "Flows",
                items: [
                    { text: "Auth", link: "/flows/auth" },
                    { text: "Realtime", link: "/flows/realtime" },
                ],
            },
            {
                text: "Operations",
                items: [
                    { text: "Single-host production", link: "/single-host-production" },
                    { text: "Backup, restore & observability", link: "/backup-restore-observability" },
                    { text: "Env reference", link: "/reference/env" },
                    { text: "Contributing & releasing", link: "/contributing-releasing" },
                ],
            },
        ],
        search: { provider: "local" },
        editLink: {
            pattern: "https://github.com/arief-fajri/pgbase/edit/main/docs/:path",
        },
        socialLinks: [{ icon: "github", link: "https://github.com/arief-fajri/pgbase" }],
    },
    markdown: {
        config: (md) => {
            // Mermaid is rendered natively on GitHub; the plugin enables preview in VitePress.
            // Loaded lazily so plain `docs/` markdown stays toolchain-free (Phase 1).
            try {
                const { mermaidPlugin } = require("vitepress-plugin-mermaid");
                md.use(mermaidPlugin);
            } catch {
                // plugin optional until `npm --prefix docs install` runs
            }
        },
    },
    vite: {
        server: { port: 5174 },
        preview: { port: 5174 },
    },
});
