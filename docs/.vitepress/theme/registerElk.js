// Registers the ELK layout engine on the singleton mermaid instance exactly
// once. ELK does orthogonal edge routing + crossing minimisation, which
// untangles the dense, multi-subgraph architecture flowchart that dagre leaves
// messy. vitepress-plugin-mermaid only calls registerExternalDiagrams, never a
// layout loader, so we must register it ourselves — and BEFORE the plugin's
// mermaid.render() runs, otherwise mermaid silently falls back to dagre.
let ready = null;

export function ensureElkRegistered() {
    if (ready) return ready;
    ready = (async () => {
        const [{ default: mermaid }, elk] = await Promise.all([
            import("mermaid"),
            import("@mermaid-js/layout-elk"),
        ]);
        // The plugin imports the same mermaid singleton, so this registration is
        // visible to its render() call.
        mermaid.registerLayoutLoaders(elk.default ?? elk);
    })();
    return ready;
}
