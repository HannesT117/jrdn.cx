const data = document.getElementById("repertoire-data");

if (data) {
    const box = data.parentElement;
    new IntersectionObserver(
        async (entries, observer) => {
            if (!entries.some((e) => e.isIntersecting)) return;
            observer.disconnect();
            const [{ mountCloud }] = await Promise.all([import("./cloud.js"), document.fonts.ready]);
            mountCloud(box, JSON.parse(data.textContent));
        },
        { rootMargin: "400px" },
    ).observe(box);
}
