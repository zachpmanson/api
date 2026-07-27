const endpoints = [
  {
    method: "GET",
    path: "/api/prod-broke",
    desc: "Latest broken deployment per environment (raw JSON).",
  },
  {
    method: "GET",
    path: "/api/prod-broke/messages",
    desc: "Human-readable “N days since last broken deployment” per environment.",
  },
];

export default function Home() {
  return (
    <main style={{ maxWidth: 640, margin: "0 auto", padding: "4rem 1.5rem" }}>
      <h1 style={{ fontSize: "1.6rem", marginBottom: "0.25rem" }}>
        zachmanson.com API
      </h1>
      <p style={{ color: "#8b93a3", marginTop: 0, lineHeight: 1.5 }}>
        A small general-purpose API backing a few of my projects. There is
        nothing at the root itself — the useful bits are the endpoints below.
      </p>

      <h2 style={{ fontSize: "1rem", marginTop: "2rem" }}>Endpoints</h2>
      <ul style={{ listStyle: "none", padding: 0 }}>
        {endpoints.map((e) => (
          <li
            key={e.path}
            style={{
              border: "1px solid #262b36",
              borderRadius: 10,
              padding: "0.8rem 1rem",
              marginBottom: "0.6rem",
            }}
          >
            <div style={{ fontFamily: "ui-monospace, Menlo, monospace" }}>
              <span style={{ color: "#34d399", fontWeight: 600 }}>
                {e.method}
              </span>{" "}
              <a
                href={e.path}
                style={{ color: "#e6e9ef", textDecoration: "none" }}
              >
                {e.path}
              </a>
            </div>
            <div
              style={{
                color: "#8b93a3",
                fontSize: "0.9rem",
                marginTop: "0.25rem",
              }}
            >
              {e.desc}
            </div>
          </li>
        ))}
      </ul>

      <p style={{ color: "#8b93a3", fontSize: "0.85rem", marginTop: "2rem" }}>
        <a href="https://github.com/zachpmanson/api" style={{ color: "#8b93a3" }}>
          github.com/zachpmanson/api
        </a>
      </p>
    </main>
  );
}
