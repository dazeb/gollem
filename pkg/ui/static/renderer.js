const hydratableSelector = '[data-renderer-root], .sidebar-fragment, .transcript';

const markHydrated = (root) => {
  if (root?.matches?.(hydratableSelector)) {
    root.setAttribute('data-renderer-ready', 'true');
  }

  root?.querySelectorAll?.(hydratableSelector).forEach((node) => {
    node.setAttribute('data-renderer-ready', 'true');
  });
};

const highlight = (root) => {
  if (window.Pretext && typeof window.Pretext.highlight === 'function') {
    window.Pretext.highlight(root || document);
  }
};

const markActiveNavigation = () => {
  const route = document.body?.dataset.route || '';
  document.querySelectorAll('.shell__nav a').forEach((link) => {
    link.classList.toggle('is-active', link.getAttribute('href') === route);
  });
};

const hydrate = (root = document) => {
  markHydrated(root);
  highlight(root);
  markActiveNavigation();
};

document.addEventListener('DOMContentLoaded', () => {
  hydrate(document);
});

document.body?.addEventListener('htmx:load', (event) => {
  hydrate(event.target || document);
});

document.body?.addEventListener('ui:fragment-loaded', (event) => {
  hydrate(event.target || document);
});
