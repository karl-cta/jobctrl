type Translations = Record<string, string>

const fr: Translations = {
  // Nav
  'nav.dashboard': 'Tableau de bord',
  'nav.applications': 'Candidatures',
  'nav.new': '+ Nouvelle',
  'nav.new_application': 'Nouvelle candidature',
  'nav.sidebar': 'Barre latérale',
  'nav.main': 'Navigation principale',
  'nav.mobile_menu': 'Menu de navigation',
  'nav.open_menu': 'Ouvrir le menu',

  // Dashboard
  'dashboard.title': 'Tableau de bord',
  'dashboard.total': 'Total candidatures',
  'dashboard.active_interviews': 'Entretiens actifs',
  'dashboard.response_rate': 'Taux de réponse',
  'dashboard.offers': 'Offres reçues',
  'dashboard.pipeline': 'Pipeline de candidature',
  'dashboard.top_sources': 'Top sources',
  'dashboard.over_time': 'Candidatures dans le temps',
  'dashboard.status_distribution': 'Répartition des statuts',
  'dashboard.no_data': 'Aucune donnée',

  // List
  'list.title': 'Candidatures',
  'list.new': '+ Nouvelle candidature',
  'list.search': 'Rechercher entreprise ou poste...',
  'list.all_statuses': 'Tous les statuts',
  'list.empty': 'Aucune candidature',
  'list.add_first': 'Ajouter la première',
  'list.confirm_delete': 'Supprimer cette candidature ?',
  'list.view_table': 'Vue tableau',
  'list.view_kanban': 'Vue kanban',
  'list.kanban_empty': 'Aucune candidature',

  // Detail
  'detail.not_found': 'Candidature introuvable',
  'detail.edit': 'Modifier',
  'detail.delete': 'Supprimer',
  'detail.contract': 'Contrat',
  'detail.mode': 'Mode',
  'detail.salary': 'Salaire',
  'detail.source': 'Source',
  'detail.notes': 'Notes',
  'detail.prep': 'Pitch / Préparation entretien',
  'detail.interviews': 'Entretiens',
  'detail.contacts': 'Contacts',
  'detail.timeline': 'Historique',
  'detail.round': 'Tour',
  'detail.confirm_delete': 'Supprimer cette candidature ?',
  'detail.tab_notes': 'Notes',
  'detail.tab_prep': 'Préparation',
  'detail.tab_interviews': 'Entretiens',
  'detail.tab_contacts': 'Contacts',
  'detail.tab_timeline': 'Historique',
  'detail.add': 'Ajouter',
  'detail.applied_at': 'Candidaté le',
  'detail.company_info': 'Entreprise',
  'detail.job_link': "Voir l'offre",
  'detail.save': 'Sauvegarder',
  'detail.confirm_delete_interview': 'Supprimer cet entretien ?',
  'detail.confirm_delete_contact': 'Supprimer ce contact ?',
  'detail.interview_add_title': 'Ajouter un entretien',
  'detail.interview_edit_title': "Modifier l'entretien",
  'detail.contact_add_title': 'Ajouter un contact',
  'detail.contact_edit_title': 'Modifier le contact',
  'detail.interview_round': 'Tour',
  'detail.interview_type': 'Type',
  'detail.interview_scheduled': 'Date / Heure',
  'detail.interview_duration': 'Durée (min)',
  'detail.interview_outcome': 'Résultat',
  'detail.interview_outcome_none': 'Pas encore',
  'detail.interview_interviewer': 'Intervieweur',
  'detail.interview_role': 'Rôle',
  'detail.interview_notes': 'Notes',
  'detail.contact_name': 'Nom',
  'detail.contact_role': 'Rôle',
  'detail.contact_email': 'Email',
  'detail.contact_phone': 'Téléphone',
  'detail.contact_linkedin': 'LinkedIn',
  'detail.contact_notes': 'Notes',
  'detail.no_notes': 'Aucune note.',
  'detail.no_prep': 'Aucune préparation.',
  'detail.no_interviews': 'Aucun entretien.',
  'detail.no_contacts': 'Aucun contact.',
  'detail.no_timeline': 'Aucun historique.',

  // Form
  'form.title_new': 'Nouvelle candidature',
  'form.title_edit': 'Modifier la candidature',
  'form.back': 'Retour',
  'form.company': 'Entreprise',
  'form.company_name': 'Entreprise',
  'form.company_website': 'Site web',
  'form.company_industry': 'Secteur',
  'form.company_size': 'Taille',
  'form.company_size_placeholder': 'ex: 50-200',
  'form.company_location': 'Localisation siège',
  'form.position': 'Poste',
  'form.job_title': 'Intitulé du poste',
  'form.job_url': "URL de l'offre",
  'form.contract_type': 'Type de contrat',
  'form.work_mode': 'Mode de travail',
  'form.location': 'Localisation poste',
  'form.source': 'Source',
  'form.source_placeholder': 'LinkedIn, Indeed, referral...',
  'form.description': 'Description du poste',
  'form.salary_status': 'Salaire & Statut',
  'form.salary_min': 'Salaire min (€/an)',
  'form.salary_max': 'Salaire max (€/an)',
  'form.status': 'Statut',
  'form.rating': 'Note (1-5)',
  'form.applied_at': 'Date de candidature',
  'form.notes_section': 'Notes & Préparation',
  'form.notes': 'Notes',
  'form.notes_placeholder': 'Observations, ressenti...',
  'form.speech': 'Pitch / Préparation entretien',
  'form.speech_placeholder': 'Votre elevator pitch, points clés à mentionner...',
  'form.cancel': 'Annuler',
  'form.save': 'Sauvegarder',
  'form.create': 'Créer la candidature',
  'form.saved': 'Candidature sauvegardée',
  'form.created': 'Candidature créée',
  'form.error': 'Une erreur est survenue',

  // Statuses
  'status.Wishlist': 'Wishlist',
  'status.Applied': 'Candidaté',
  'status.Screening': 'Présélection',
  'status.Interviewing': 'Entretiens',
  'status.Offer': 'Offre reçue',
  'status.Accepted': 'Accepté',
  'status.Rejected': 'Refusé',
  'status.Withdrawn': 'Retiré',

  // Common
  'common.page_not_found': 'Page non trouvée',
  'common.theme_toggle': 'Changer de thème',
  'common.close': 'Fermer',
  'common.skip_to_content': 'Aller au contenu',
  'common.search': 'Rechercher',
  'common.filter_status': 'Filtrer par statut',
}

const en: Translations = {
  // Nav
  'nav.dashboard': 'Dashboard',
  'nav.applications': 'Applications',
  'nav.new': '+ New',
  'nav.new_application': 'New application',
  'nav.sidebar': 'Sidebar',
  'nav.main': 'Main navigation',
  'nav.mobile_menu': 'Navigation menu',
  'nav.open_menu': 'Open menu',

  // Dashboard
  'dashboard.title': 'Dashboard',
  'dashboard.total': 'Total applications',
  'dashboard.active_interviews': 'Active interviews',
  'dashboard.response_rate': 'Response rate',
  'dashboard.offers': 'Offers received',
  'dashboard.pipeline': 'Application pipeline',
  'dashboard.top_sources': 'Top sources',
  'dashboard.over_time': 'Applications over time',
  'dashboard.status_distribution': 'Status distribution',
  'dashboard.no_data': 'No data',

  // List
  'list.title': 'Applications',
  'list.new': '+ New application',
  'list.search': 'Search company or position...',
  'list.all_statuses': 'All statuses',
  'list.empty': 'No applications',
  'list.add_first': 'Add the first one',
  'list.confirm_delete': 'Delete this application?',
  'list.view_table': 'Table view',
  'list.view_kanban': 'Kanban view',
  'list.kanban_empty': 'No applications',

  // Detail
  'detail.not_found': 'Application not found',
  'detail.edit': 'Edit',
  'detail.delete': 'Delete',
  'detail.contract': 'Contract',
  'detail.mode': 'Mode',
  'detail.salary': 'Salary',
  'detail.source': 'Source',
  'detail.notes': 'Notes',
  'detail.prep': 'Pitch / Interview preparation',
  'detail.interviews': 'Interviews',
  'detail.contacts': 'Contacts',
  'detail.timeline': 'Timeline',
  'detail.round': 'Round',
  'detail.confirm_delete': 'Delete this application?',
  'detail.tab_notes': 'Notes',
  'detail.tab_prep': 'Prep',
  'detail.tab_interviews': 'Interviews',
  'detail.tab_contacts': 'Contacts',
  'detail.tab_timeline': 'Timeline',
  'detail.add': 'Add',
  'detail.applied_at': 'Applied on',
  'detail.company_info': 'Company',
  'detail.job_link': 'View job',
  'detail.save': 'Save',
  'detail.confirm_delete_interview': 'Delete this interview?',
  'detail.confirm_delete_contact': 'Delete this contact?',
  'detail.interview_add_title': 'Add interview',
  'detail.interview_edit_title': 'Edit interview',
  'detail.contact_add_title': 'Add contact',
  'detail.contact_edit_title': 'Edit contact',
  'detail.interview_round': 'Round',
  'detail.interview_type': 'Type',
  'detail.interview_scheduled': 'Date / Time',
  'detail.interview_duration': 'Duration (min)',
  'detail.interview_outcome': 'Outcome',
  'detail.interview_outcome_none': 'Not yet',
  'detail.interview_interviewer': 'Interviewer',
  'detail.interview_role': 'Role',
  'detail.interview_notes': 'Notes',
  'detail.contact_name': 'Name',
  'detail.contact_role': 'Role',
  'detail.contact_email': 'Email',
  'detail.contact_phone': 'Phone',
  'detail.contact_linkedin': 'LinkedIn',
  'detail.contact_notes': 'Notes',
  'detail.no_notes': 'No notes yet.',
  'detail.no_prep': 'No prep notes yet.',
  'detail.no_interviews': 'No interviews yet.',
  'detail.no_contacts': 'No contacts yet.',
  'detail.no_timeline': 'No activity yet.',

  // Form
  'form.title_new': 'New application',
  'form.title_edit': 'Edit application',
  'form.back': 'Back',
  'form.company': 'Company',
  'form.company_name': 'Company',
  'form.company_website': 'Website',
  'form.company_industry': 'Industry',
  'form.company_size': 'Size',
  'form.company_size_placeholder': 'e.g. 50-200',
  'form.company_location': 'HQ location',
  'form.position': 'Position',
  'form.job_title': 'Job title',
  'form.job_url': 'Job URL',
  'form.contract_type': 'Contract type',
  'form.work_mode': 'Work mode',
  'form.location': 'Job location',
  'form.source': 'Source',
  'form.source_placeholder': 'LinkedIn, Indeed, referral...',
  'form.description': 'Job description',
  'form.salary_status': 'Salary & Status',
  'form.salary_min': 'Min salary (€/yr)',
  'form.salary_max': 'Max salary (€/yr)',
  'form.status': 'Status',
  'form.rating': 'Rating (1-5)',
  'form.applied_at': 'Application date',
  'form.notes_section': 'Notes & Preparation',
  'form.notes': 'Notes',
  'form.notes_placeholder': 'Observations, impressions...',
  'form.speech': 'Pitch / Interview preparation',
  'form.speech_placeholder': 'Your elevator pitch, key points to mention...',
  'form.cancel': 'Cancel',
  'form.save': 'Save',
  'form.create': 'Create application',
  'form.saved': 'Application saved',
  'form.created': 'Application created',
  'form.error': 'An error occurred',

  // Statuses
  'status.Wishlist': 'Wishlist',
  'status.Applied': 'Applied',
  'status.Screening': 'Screening',
  'status.Interviewing': 'Interviewing',
  'status.Offer': 'Offer',
  'status.Accepted': 'Accepted',
  'status.Rejected': 'Rejected',
  'status.Withdrawn': 'Withdrawn',

  // Common
  'common.page_not_found': 'Page not found',
  'common.theme_toggle': 'Toggle theme',
  'common.close': 'Close',
  'common.skip_to_content': 'Skip to content',
  'common.search': 'Search',
  'common.filter_status': 'Filter by status',
}

const locales: Record<string, Translations> = { fr, en }

let currentLocale = localStorage.getItem('jc-locale') || 'fr'

export function t(key: string): string {
  return locales[currentLocale]?.[key] ?? locales.fr[key] ?? key
}

export function setLocale(locale: string) {
  currentLocale = locale
  localStorage.setItem('jc-locale', locale)
}

export function getLocale(): string {
  return currentLocale
}

export function getDateLocale(): string {
  return currentLocale === 'fr' ? 'fr-FR' : 'en-US'
}

export function translateTimelineEvent(eventType: string, description: string): string {
  if (currentLocale === 'en') return description
  switch (eventType) {
    case 'created': return 'Candidature créée'
    case 'interview_deleted': return 'Entretien supprimé'
    case 'contact_deleted': return 'Contact supprimé'
    case 'status_change': {
      const m = description.match(/from (\w+) to (\w+)/)
      if (m) return `${t('status.' + m[1])} → ${t('status.' + m[2])}`
      return description
    }
    case 'interview_added': {
      const m = description.match(/round (\d+) \(([^)]+)\)/)
      if (m) return `Entretien tour ${m[1]} (${m[2]}) ajouté`
      return description
    }
    case 'contact_added': {
      const m = description.match(/Contact (.+) added/)
      if (m) return `Contact ${m[1]} ajouté`
      return description
    }
    default: return description
  }
}
