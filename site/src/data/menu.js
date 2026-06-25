// src/data/menu.js

// 1. Top Level Navigation (Home, About, Learn, etc.)
export const navItems = [
  { id: 'home', label: 'HOME', href: '/' },
  { 
    id: 'about', 
    label: 'ABOUT', 
    dropdown: [
      { label: 'Using the ideas library', href: '/about/using' },
      { label: 'History of L\'Abri', href: '/about/history' }
    ]
  },
  { 
    id: 'learn', 
    label: 'LEARN', 
    dropdown: [
      { label: 'Topics', href: '/topics/' },
      { label: 'Featured', href: '/featured/' },
      { label: 'Playlists', href: '/playlists/' },
      { label: 'Schaeffer Collection', href: '/schaeffer/' },
      { label: 'Search', href: '/search/search2/' },
      { label: 'Search-base', href: '/search/search' },
      { label: 'Search-ranked', href: '/search/search-ranked/' },
      { label: 'Search-scoped', href: '/search/search-scoped/' }
    ]
  },
  { id: 'contact', 
    label: 'CONTACT', 
    dropdown: [
      { label: 'Contact L\'Abri', href: '/contact/contact' },
      { label: 'Giving', href: '/contact/giving' },
      { label: 'Report An Error', href: '/contact/Report An Error' }
    ]
  },
  { id: 'external', label: 'L\'ABRI.ORG', href: 'https://labri.org' },
  { id: 'dev', label: 'DEV', href: '/admin' }
];
