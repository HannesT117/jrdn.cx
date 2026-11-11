import { defineConfig } from "vite";

export default defineConfig({
    publicDir: "static",
    server: {
        proxy: {
            "/api/query": "http://localhost:8787",
            "/api/speech": "http://localhost:8787",
            "/api/extras": "http://localhost:8787",
            "/api/activity": "http://localhost:8787",
        },
    },
    build: {
        rollupOptions: {
            input: { main: "index.html", imprint: "imprint.html" },
        },
    },
});
