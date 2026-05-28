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
  { id: 'learn', label: 'LEARN', href: '/learn' },
  { id: 'contact', label: 'CONTACT', href: '/contact' },
  { id: 'external', label: 'L\'ABRI.ORG', href: 'https://labri.org' }
];

// 2. Mega Menu Taxonomy (Worldview, Culture, etc.)
export const menuStructure = {
  WORLDVIEW: {
    // Left & Middle Columns: The navigation taxonomy
    categories: {
      "L'Abri": ["Introduction", "Core Ideas"],
      "Bible": ["Old Testament", "New Testament", "Canon", "Interpretation", "Inspiration"],
      "Doctrine": ["Jesus", "Holy Spirit", "Big Questions", "Being Human", "Salvation", "Spiritual Growth", "Prayer & Worship", "Law", "Sin & Evil", "Prophecy & Future", "Church & Sacraments"],
      "Apologetics": ["Exploring Christianity", "Sharing Your Faith", "Common Questions"],
      "History": ["Church History", "Liberalism", "Catholicism"],
      "More...": ["Ethics & Morality", "Cults & the Occult", "Islam", "Eastern Religion", "New Age"]
    },
    // Right Column: Static Featured Lectures
    featured: [
      { title: "The Five Themes of L'Abri", speaker: "Dick Keyes", url: "/lectures/themes" },
      { title: "An Introduction to Francis Schaeffer", speaker: "Jerram Barrs", url: "/lectures/intro-schaeffer" },
      { title: "Comprehensive Spirituality", speaker: "Ellis Potter", url: "/lectures/comprehensive-spirituality" }
    ]
  },
  
  CULTURE: {
    categories: {
      "Arts": ["Theology", "Film", "Literature & Poetry", "Modern Art", "Theatre & Stage", "Classical Music", "Popular Music", "Music: General"],
      "Modernity": ["Christianity & Modernity", "Christianity & Culture"],
      "Science": ["Science & Faith"],
      "Technology": ["Technology & the Future"],
      "Philosophy": ["Christianity & Philosophy"],
      "More...": ["Leisure & Games", "Education", "War", "Economics", "Race", "Work", "Medical Ethics", "Ecology", "History of Thought", "Politics"]
    },
    featured: [
      { title: "Art & The Bible", speaker: "Francis Schaeffer", url: "/lectures/art-bible" },
      { title: "Les Misérables", speaker: "John Hodges", url: "/lectures/les-miserables" },
      { title: "On Saving Leonardo", speaker: "Nancy Pearcey", url: "/lectures/saving-leonardo" }
    ]
  },
  
  PERSONAL: {
    categories: {
      "Relationships": ["Community", "Hospitality", "Marriage", "Singleness", "Family"],
      "Self": ["Identity", "Psychology", "Emotions"],
      "Sex & Gender": ["Christianity & Sexuality"],
      "Suffering": ["Suffering, Pain, and Healing"],
      "More...": ["Making Decisions", "Food", "Ethics"]
    },
    featured: [
      { title: "What is a 'Personal Relationship with God'?", speaker: "Jim Paul", url: "/lectures/personal-relationship" },
      { title: "Community As A Subversion of Modernity", speaker: "Andrew Fellows", url: "/lectures/community-subversion" },
      { title: "Navigating Relationships", speaker: "Jerram Barrs", url: "/lectures/navigating-relationships" }
    ]
  }
};
