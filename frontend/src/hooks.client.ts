// After redeployment, the browser may reference old chunk hashes
// that no longer exist on the server. Reload to get fresh assets.
window.addEventListener('vite:preloadError', () => {
	window.location.reload();
});
