---
title: gohawk
description: High-signal static analysis for Go.
template: splash
tableOfContents: false
editUrl: false
---

<div class="landing">

<div class="landing-hero">
  <figure class="landing-figure">
    <img src="/gohawk/gohawk-logo.png" width="1536" height="1024" alt="A hawk sheltering the Go gopher" />
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
  <div class="source-window">
    <div class="source-window-bar" aria-hidden="true">
      <span class="source-window-file">worker.go</span>
    </div>
    <div class="source-window-body" aria-label="Example Go source code">
      <div><span class="source-line-number">1</span><span class="source-keyword">package</span> worker</div>
      <div><span class="source-line-number">2</span></div>
      <div><span class="source-line-number">3</span><span class="source-keyword">func</span> run(jobs []Job) {</div>
      <div><span class="source-line-number">4</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">var</span> err <span class="source-type">error</span></div>
      <div><span class="source-line-number">5</span>&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">for</span> _, job := <span class="source-keyword">range</span> jobs {</div>
      <div><span class="source-line-number">6</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<span class="source-keyword">go</span> <span class="source-keyword">func</span>() {</div>
      <div><span class="source-line-number">7</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;prepare(job)</div>
      <div class="source-line-highlight"><span class="source-line-number">8</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;err = fetch()</div>
      <div><span class="source-line-number">9</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;}()</div>
      <div><span class="source-line-number">10</span>&nbsp;&nbsp;&nbsp;&nbsp;}</div>
      <div><span class="source-line-number">11</span>}</div>
    </div>
  </div>
  <div class="terminal">
    <div class="terminal-bar" aria-hidden="true">
      <span class="terminal-dot" data-dot="close"></span>
      <span class="terminal-dot" data-dot="min"></span>
      <span class="terminal-dot" data-dot="max"></span>
    </div>
    <div class="terminal-body">
      <div><span class="terminal-prompt">$</span> <span class="terminal-cmd">go install github.com/kojah/gohawk@latest</span></div>
      <div><span class="terminal-prompt">$</span> <span class="terminal-cmd">gohawk ./...</span></div>
      <div class="terminal-gap"></div>
      <div><span class="terminal-warn">warning</span>[<span class="terminal-label">concurrentcapture</span>]: <span class="terminal-cmd">captured local err is mutated by goroutines launched repeatedly</span></div>
      <div>&nbsp; <span class="terminal-prompt">--&gt;</span> worker.go:8:4</div>
      <div>&nbsp; <span class="terminal-prompt">|</span></div>
      <div><span class="terminal-prompt">8 |</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; err = fetch()</div>
      <div>&nbsp; <span class="terminal-prompt">|</span>&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; <span class="terminal-marker">^~~</span></div>
    </div>
  </div>
</div>

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

</div>
