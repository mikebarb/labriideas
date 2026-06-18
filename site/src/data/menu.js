// src/data/menu.js

// 1. Top Level Navigation (Home, About, Learn, etc.)
export const navItems = [
  { id: 'home', label: 'HOME', href: '/' },
  { 
    id: 'about', 
    label: 'ABOUT', 
    href: '/about', 
    dropdown: [
      { label: 'Using the ideas library', href: '/about/using' },
      { label: 'History of L\'Abri', href: '/about/history' }
    ]
  },
  { 
    id: 'learn', 
    label: 'LEARN', 
    href: '/learn',
    dropdown: [
      { label: 'Topics', href: '/topics/' },
      { label: 'Playlists', href: '/playlists/' },
      { label: 'Featured Lectures', href: '/featured/' }
    ]
  },
  { id: 'contact', label: 'CONTACT', href: '/contact' },
  { id: 'external', label: 'L\'ABRI.ORG', href: 'https://labri.org' },
  { id: 'dev', label: 'DEV', href: '/admin' }
];
