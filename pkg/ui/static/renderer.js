const hydrateSidebarState = (root = document) => {
  root.querySelectorAll('[data-renderer-root], .sidebar-fragment, .transcript').forEach((node) => {
    node.setAttribute('data-renderer-ready', 'true');
  });

  if (window.Pretext && typeof window.Pretext.highlight === 'function') {
    window.Pretext.highlight(root);
  }
};

const markActiveNavigation = () => {
  const route = document.body?.dataset.route || '';
  document.querySelectorAll('.shell__nav a').forEach((link) => {
    if (link.getAttribute('href') === route) {
      link.classList.add('is-active');
    }
  });
};

document.addEventListener('DOMContentLoaded', () => {
  hydrateSidebarState(document);
  markActiveNavigation();
});

document.body?.addEventListener('ui:fragment-loaded', () => {
  hydrateSidebarState(document);
});
