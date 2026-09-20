import './style.css';

const app = document.querySelector<HTMLDivElement>('#app')!;

app.innerHTML = `
  <main class="shell">
    <header class="topbar">
      <div class="brand">
        <span class="brand-mark">PT</span>
        <span class="brand-name">PartsTable <span class="brand-sub">Connector</span></span>
      </div>
      <span class="vintage" id="vintage">compendium rev —</span>
    </header>
    <section class="hero">
      <h1>Look up any part.</h1>
      <p class="lede">
        Paste a whole list. Get the description, the substitutes, and the
        source of every fact.
      </p>
      <p class="muted">
        The compendium loads with your first release build. Every answer comes
        from your own machine — nothing you look up leaves it.
      </p>
    </section>
  </main>
`;
