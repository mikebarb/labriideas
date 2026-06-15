import { defineMiddleware } from 'astro:middleware';

export const onRequest = defineMiddleware(async (context, next) => {
  const url = context.url;
  // Check if the URL is a request for a folder route (e.g., /admin, /catalogViewer) without a trailing slash
  if (
    url.pathname !== '/' && 
    !url.pathname.endsWith('/') && 
    !url.pathname.includes('.') // ignore files like .css, .js, .png
  ) {
    // Rewrite internally to add the trailing slash, avoiding the warning and the 301 redirect
    return context.rewrite(url.pathname + '/');
  }
  return next();
});
