---
title: gohawk
description: "Find concurrency and resource management bugs at compile-time using gohawk’s advanced suite of static analyzers."
template: splash
tableOfContents: false
editUrl: false
head:
  - tag: title
    content: "gohawk | Resource-focused static analysis for Go"
  - tag: meta
    attrs:
      property: og:title
      content: "gohawk | Resource-focused static analysis for Go"
  - tag: meta
    attrs:
      name: twitter:title
      content: "gohawk | Resource-focused static analysis for Go"
  - tag: meta
    attrs:
      name: twitter:description
      content: "Find concurrency and resource management bugs at compile-time using gohawk’s advanced suite of static analyzers."
---

<div id="_top" class="landing">

<div class="landing-hero">
  <figure class="landing-figure">
    <img src="/gohawk-logo.png" width="1536" height="1024" alt="A hawk sheltering the Go gopher" />
  </figure>
  <div class="landing-copy">
    <h1 class="landing-title">gohawk</h1>
    <p class="landing-tagline">Watch for bugs like a hawk!</p>
    <p class="landing-body">gohawk is a correctness-oriented suite of Go analyzers. It uses SSA-backed
    static analysis to catch bugs based on deep understanding of a program's control flow and
    resource lifecycles.</p>
    <p class="landing-get-started-row"><a class="landing-get-started" href="installation/">Get started <span aria-hidden="true">&rarr;</span></a></p>
  </div>
</div>

<div class="landing-demo">
  <div class="source-carousel" data-analyzer-carousel role="region" aria-roledescription="carousel" aria-label="Examples of bugs found by gohawk">
    <button class="source-carousel-arrow" type="button" data-carousel-previous aria-label="Show previous example">&lsaquo;</button>
    <div class="source-card">
    <div class="source-slide" data-carousel-slide aria-label="1 of 4: contradictory lock order">
      <div class="source-window-bar">
        <span class="source-window-file">cache.go</span>
        <span class="source-analyzer">lockorder</span>
      </div>
      <div class="source-window-body" aria-label="Go code with locks acquired in contradictory order">
        <div><span class="source-line-number">1</span><span class="source-keyword">func</span> refresh() {</div>
        <div><span class="source-line-number">2</span>&nbsp;&nbsp;index.Lock()</div>
        <div><span class="source-line-number">3</span>&nbsp;&nbsp;<span class="source-keyword">defer</span> index.Unlock()</div>
        <div><span class="source-line-number">4</span>&nbsp;&nbsp;cache.Lock()</div>
        <div><span class="source-line-number">5</span>&nbsp;&nbsp;<span class="source-keyword">defer</span> cache.Unlock()</div>
        <div><span class="source-line-number">6</span>}</div>
        <div><span class="source-line-number">7</span></div>
        <div><span class="source-line-number">8</span><span class="source-keyword">func</span> reset() {</div>
        <div><span class="source-line-number">9</span>&nbsp;&nbsp;cache.Lock()</div>
        <div><span class="source-line-number">10</span>&nbsp;&nbsp;<span class="source-keyword">defer</span> cache.Unlock()</div>
        <div class="source-line-highlight"><span class="source-line-number">11</span>&nbsp;&nbsp;index.Lock()</div>
        <div><span class="source-line-number">12</span>&nbsp;&nbsp;<span class="source-keyword">defer</span> index.Unlock()</div>
        <div><span class="source-line-number">13</span>}</div>
      </div>
      <div class="source-finding">
        <span class="source-finding-icon" aria-hidden="true">!</span>
        <span><strong>lockorder</strong>contradictory lock order: index and cache</span>
      </div>
    </div>
    <div class="source-slide" data-carousel-slide aria-label="2 of 4: resource lifetime leak" hidden>
      <div class="source-window-bar">
        <span class="source-window-file">config.go</span>
        <span class="source-analyzer">resourcelifetime</span>
      </div>
      <div class="source-window-body" aria-label="Go code that leaks an open file">
        <div><span class="source-line-number">1</span><span class="source-keyword">func</span> load(path <span class="source-type">string</span>) (<span class="source-type">Config</span>, <span class="source-type">error</span>) {</div>
        <div class="source-line-highlight"><span class="source-line-number">2</span>&nbsp;&nbsp;file, err := os.Open(path)</div>
        <div><span class="source-line-number">3</span>&nbsp;&nbsp;<span class="source-keyword">if</span> err != <span class="source-keyword">nil</span> {</div>
        <div><span class="source-line-number">4</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">return</span> Config{}, err</div>
        <div><span class="source-line-number">5</span>&nbsp;&nbsp;}</div>
        <div><span class="source-line-number">6</span></div>
        <div><span class="source-line-number">7</span>&nbsp;&nbsp;<span class="source-keyword">var</span> config Config</div>
        <div><span class="source-line-number">8</span>&nbsp;&nbsp;<span class="source-keyword">if</span> err := json.NewDecoder(file).Decode(&amp;config); err != <span class="source-keyword">nil</span> {</div>
        <div><span class="source-line-number">9</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">return</span> Config{}, err</div>
        <div><span class="source-line-number">10</span>&nbsp;&nbsp;}</div>
        <div><span class="source-line-number">11</span>&nbsp;&nbsp;<span class="source-keyword">return</span> config, <span class="source-keyword">nil</span></div>
        <div><span class="source-line-number">12</span>}</div>
        <div><span class="source-line-number">13</span></div>
      </div>
      <div class="source-finding">
        <span class="source-finding-icon" aria-hidden="true">!</span>
        <span><strong>resourcelifetime</strong>owned resource from os.Open is not released on every return path</span>
      </div>
    </div>
    <div class="source-slide" data-carousel-slide aria-label="3 of 4: unjoined goroutine" hidden>
      <div class="source-window-bar">
        <span class="source-window-file">refresh.go</span>
        <span class="source-analyzer">goroutineownership</span>
      </div>
      <div class="source-window-body" aria-label="Go code with a goroutine that is not joined on an error path">
        <div><span class="source-line-number">1</span><span class="source-keyword">func</span> refresh() <span class="source-type">error</span> {</div>
        <div><span class="source-line-number">2</span>&nbsp;&nbsp;done := make(<span class="source-keyword">chan</span> <span class="source-type">struct</span>{})</div>
        <div class="source-line-highlight"><span class="source-line-number">3</span>&nbsp;&nbsp;<span class="source-keyword">go</span> <span class="source-keyword">func</span>() {</div>
        <div><span class="source-line-number">4</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">defer</span> close(done)</div>
        <div><span class="source-line-number">5</span>&nbsp;&nbsp;&nbsp;&nbsp;updateCache()</div>
        <div><span class="source-line-number">6</span>&nbsp;&nbsp;}()</div>
        <div><span class="source-line-number">7</span></div>
        <div><span class="source-line-number">8</span>&nbsp;&nbsp;<span class="source-keyword">if</span> err := syncIndex(); err != <span class="source-keyword">nil</span> {</div>
        <div><span class="source-line-number">9</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">return</span> err</div>
        <div><span class="source-line-number">10</span>&nbsp;&nbsp;}</div>
        <div><span class="source-line-number">11</span>&nbsp;&nbsp;&lt;-done</div>
        <div><span class="source-line-number">12</span>&nbsp;&nbsp;<span class="source-keyword">return</span> <span class="source-keyword">nil</span></div>
        <div><span class="source-line-number">13</span>}</div>
      </div>
      <div class="source-finding">
        <span class="source-finding-icon" aria-hidden="true">!</span>
        <span><strong>goroutineownership</strong>goroutine is not joined on every return path</span>
      </div>
    </div>
    <div class="source-slide" data-carousel-slide aria-label="4 of 4: concurrent capture" hidden>
      <div class="source-window-bar">
        <span class="source-window-file">worker.go</span>
        <span class="source-analyzer">concurrentcapture</span>
      </div>
      <div class="source-window-body" aria-label="Go code with goroutines mutating the same captured local">
        <div><span class="source-line-number">1</span><span class="source-keyword">func</span> collect(items []Item) <span class="source-type">error</span> {</div>
        <div><span class="source-line-number">2</span>&nbsp;&nbsp;<span class="source-keyword">var</span> err <span class="source-type">error</span></div>
        <div><span class="source-line-number">3</span>&nbsp;&nbsp;<span class="source-keyword">for</span> _, item := <span class="source-keyword">range</span> items {</div>
        <div><span class="source-line-number">4</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">go</span> <span class="source-keyword">func</span>() {</div>
        <div><span class="source-line-number">5</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;prepare(item)</div>
        <div class="source-line-highlight"><span class="source-line-number">6</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;err = fetch(item)</div>
        <div><span class="source-line-number">7</span>&nbsp;&nbsp;&nbsp;&nbsp;}()</div>
        <div><span class="source-line-number">8</span>&nbsp;&nbsp;}</div>
        <div><span class="source-line-number">9</span></div>
        <div><span class="source-line-number">10</span>&nbsp;&nbsp;<span class="source-keyword">return</span> err</div>
        <div><span class="source-line-number">11</span>}</div>
        <div><span class="source-line-number">12</span></div>
        <div><span class="source-line-number">13</span></div>
      </div>
      <div class="source-finding">
        <span class="source-finding-icon" aria-hidden="true">!</span>
        <span><strong>concurrentcapture</strong>captured local err is mutated by goroutines launched repeatedly</span>
      </div>
    </div>
    </div>
    <button class="source-carousel-arrow" type="button" data-carousel-next aria-label="Show next example">&rsaquo;</button>
    <div class="source-carousel-controls">
      <div class="source-carousel-dots" aria-label="Choose an example">
        <button type="button" data-carousel-dot aria-label="Show lock order example" aria-current="true"></button>
        <button type="button" data-carousel-dot aria-label="Show resource lifetime example"></button>
        <button type="button" data-carousel-dot aria-label="Show goroutine ownership example"></button>
        <button type="button" data-carousel-dot aria-label="Show concurrent capture example"></button>
      </div>
    </div>
  </div>
</div>

<script>
  const carousel = document.querySelector('[data-analyzer-carousel]');

  if (carousel instanceof HTMLElement && carousel.dataset.ready !== 'true') {
    carousel.dataset.ready = 'true';

    const slides = Array.from(carousel.querySelectorAll('[data-carousel-slide]'));
    const dots = Array.from(carousel.querySelectorAll('[data-carousel-dot]'));
    const previous = carousel.querySelector('[data-carousel-previous]');
    const next = carousel.querySelector('[data-carousel-next]');
    let current = 0;
    let timer;

    const show = (index) => {
      current = (index + slides.length) % slides.length;
      slides.forEach((slide, slideIndex) => {
        if (slide instanceof HTMLElement) slide.hidden = slideIndex !== current;
      });
      dots.forEach((dot, dotIndex) => {
        if (!(dot instanceof HTMLElement)) return;
        if (dotIndex === current) dot.setAttribute('aria-current', 'true');
        else dot.removeAttribute('aria-current');
      });
    };

    const stop = () => window.clearInterval(timer);
    const start = () => {
      stop();
      if (document.hidden) return;
      timer = window.setInterval(() => show(current + 1), 6500);
    };
    const select = (index) => {
      show(index);
      start();
    };

    previous?.addEventListener('click', () => select(current - 1));
    next?.addEventListener('click', () => select(current + 1));
    dots.forEach((dot, index) => dot.addEventListener('click', () => select(index)));
    carousel.addEventListener('mouseenter', stop);
    carousel.addEventListener('mouseleave', start);
    carousel.addEventListener('focusin', stop);
    carousel.addEventListener('focusout', (event) => {
      if (!carousel.contains(event.relatedTarget)) start();
    });
    document.addEventListener('visibilitychange', start);
    start();
  }
</script>

<div class="landing-orders">
  <div class="order">
    <h3>Better together</h3>
    <p>gohawk complements your existing analyzers such as <code>go vet</code>, Staticcheck, and
    go-critic; it does not try to replace them.</p>
  </div>
  <div class="order">
    <h3>Easy to drop in</h3>
    <p>gohawk runs as a standalone tool or as a <code>go vet</code> tool. No configuration file,
    and every check is a command-line flag.</p>
  </div>
  <div class="order">
    <h3>Analysis features</h3>
    <p>gohawk analyzers are primarily focused on enforcing reliability around resource management,
    concurrency, and error handling.</p>
  </div>
  <div class="order">
    <h3>Rich diagnostics</h3>
    <p>Each finding pinpoints the offending code with a precise source span and includes a suggested
    fix when gohawk can provide one safely.</p>
  </div>
</div>

<footer class="landing-links">
  <span class="landing-links-label">Explore gohawk</span>
  <nav aria-label="Explore gohawk">
    <a href="/installation/">Installation</a>
    <a href="/analyzers/">Analyzers</a>
    <a href="/configuration/">Configuration</a>
    <a href="/golangci-lint/">golangci-lint</a>
    <a href="/faq/">FAQ</a>
  </nav>
</footer>

</div>
