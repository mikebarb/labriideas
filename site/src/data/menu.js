// src/data/menu.js

// 1. Top Level Navigation (Home, About, Learn, etc.)
export const navItems = [
  { id: 'home', label: 'HOME', href: '/' },
  { 
    id: 'about', 
    label: 'ABOUT', 
    dropdown: [
      { label: 'Using the Ideas Library', href: '/about/using' },
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
      { label: 'Search', href: '/search/search3/' }
    ]
  },
  { id: 'contact', 
    label: 'CONTACT', 
    dropdown: [
      { label: 'Contact L\'Abri', href: '/contact/contact' },
      { label: 'Giving', href: '/contact/giving' },
      { label: 'Report An Error', href: 'https://labriideaslibrary.typeform.com/to/cYY0DA' }
    ]
  },
  { id: 'external', label: 'L\'ABRI.ORG', href: 'https://labri.org' },
  { id: 'misc', 
      label: 'Misc', 
      dropdown: [
        { label: 'Search', href: '/search/search3/' },
        { label: 'Search-base', href: '/searches/search/' },
        { label: 'Search-ranked', href: '/searches/search-ranked/' },
        { label: 'Search-scoped', href: '/searches/search-scoped/' },
        { label: 'Search-filtered', href: '/searches/search2/' },
        { label: 'Report An Error', href: 'https://labriideaslibrary.typeform.com/to/cYY0DA' },
        { label: 'Admin', href: '/admin' }
      ]
    }
  ];