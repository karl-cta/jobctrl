export interface JobBoard {
  name: string
  domain?: string
  aliases?: string[]
}

export const JOB_BOARDS: JobBoard[] = [
  // ── Global generalist ──
  { name: 'LinkedIn', domain: 'linkedin.com' },
  { name: 'Indeed', domain: 'indeed.com' },
  { name: 'Glassdoor', domain: 'glassdoor.com' },
  { name: 'Monster', domain: 'monster.com' },
  { name: 'ZipRecruiter', domain: 'ziprecruiter.com' },
  { name: 'CareerBuilder', domain: 'careerbuilder.com' },
  { name: 'SimplyHired', domain: 'simplyhired.com' },
  { name: 'Jooble', domain: 'jooble.org' },
  { name: 'Adzuna', domain: 'adzuna.com' },
  { name: 'Talent.com', domain: 'talent.com' },
  { name: 'Snagajob', domain: 'snagajob.com' },
  { name: 'Jobrapido', domain: 'jobrapido.com' },
  { name: 'Trovit', domain: 'trovit.com' },
  { name: 'Careerjet', domain: 'careerjet.com' },
  { name: 'Jobberman', domain: 'jobberman.com' },

  // ── Tech / Startup ──
  { name: 'Wellfound', domain: 'wellfound.com', aliases: ['angellist', 'angel list'] },
  { name: 'Dice', domain: 'dice.com' },
  { name: 'Stack Overflow Jobs', domain: 'stackoverflow.com', aliases: ['stackoverflow'] },
  { name: 'GitHub Jobs', domain: 'github.com', aliases: ['github'] },
  { name: 'Hired', domain: 'hired.com' },
  { name: 'Otta', domain: 'otta.com' },
  { name: 'Cord', domain: 'cord.co' },
  { name: 'Honeypot', domain: 'honeypot.io' },
  { name: 'Landing.jobs', domain: 'landing.jobs' },
  { name: 'BuiltIn', domain: 'builtin.com', aliases: ['built in'] },
  { name: 'Levels.fyi', domain: 'levels.fyi', aliases: ['levels'] },
  { name: 'Hacker News', domain: 'news.ycombinator.com', aliases: ['hn', 'who is hiring', 'ycombinator'] },
  { name: 'Key Values', domain: 'keyvalues.com' },
  { name: 'DevITjobs', domain: 'devitjobs.com' },
  { name: 'Relocate.me', domain: 'relocate.me' },
  { name: 'Arc.dev', domain: 'arc.dev' },
  { name: 'Gun.io', domain: 'gun.io' },
  { name: 'Triplebyte', domain: 'triplebyte.com' },
  { name: 'Underdog.io', domain: 'underdog.io' },
  { name: 'AngelList', domain: 'angel.co', aliases: ['angel.co'] },
  { name: 'Y Combinator Jobs', domain: 'workatastartup.com', aliases: ['work at a startup', 'yc jobs'] },
  { name: 'Sequoia Jobs', domain: 'jobs.sequoiacap.com' },

  // ── Tech / IT specialized ──
  { name: 'Turing', domain: 'turing.com' },
  { name: 'Crossover', domain: 'crossover.com' },
  { name: 'Andela', domain: 'andela.com' },
  { name: 'Terminal', domain: 'terminal.io' },
  { name: 'X-Team', domain: 'x-team.com', aliases: ['xteam'] },
  { name: 'Braintrust', domain: 'usebraintrust.com' },
  { name: 'A.Team', domain: 'a.team' },
  { name: 'Karat', domain: 'karat.com' },
  { name: 'Vettery', domain: 'vettery.com' },
  { name: 'Authentic Jobs', domain: 'authenticjobs.com' },
  { name: 'Smashing Jobs', domain: 'jobs.smashingmagazine.com', aliases: ['smashing magazine'] },
  { name: 'Dribbble Jobs', domain: 'dribbble.com', aliases: ['dribbble'] },
  { name: 'Behance Jobs', domain: 'behance.net', aliases: ['behance'] },
  { name: 'Just Django Jobs', domain: 'justdjango.com', aliases: ['django'] },
  { name: 'RubyNow', domain: 'rubynow.com' },
  { name: 'GoLang Jobs', domain: 'golangprojects.com', aliases: ['golang'] },
  { name: 'Rust Jobs', domain: 'rustjobs.dev' },
  { name: 'Python.org Jobs', domain: 'python.org', aliases: ['python jobs'] },
  { name: 'JSJobbs', domain: 'jsjobbs.com', aliases: ['javascript jobs'] },
  { name: 'iOS Dev Jobs', domain: 'iosdevjobs.com' },
  { name: 'Android Jobs', domain: 'androidjobs.io' },
  { name: 'AI Jobs', domain: 'ai-jobs.net', aliases: ['artificial intelligence jobs'] },
  { name: 'ML Jobs', domain: 'mljobs.io', aliases: ['machine learning jobs'] },
  { name: 'DataJobs', domain: 'datajobs.com', aliases: ['data science jobs'] },
  { name: 'Kaggle Jobs', domain: 'kaggle.com', aliases: ['kaggle'] },
  { name: 'DevOps Jobs', domain: 'devopsjobs.net' },
  { name: 'InfoSec Jobs', domain: 'infosec-jobs.com', aliases: ['cybersecurity jobs', 'security jobs'] },
  { name: 'CyberSecJobs', domain: 'cybersecjobs.com' },
  { name: 'Blockchain Works', domain: 'blockchain.works' },
  { name: 'Product Hunt Jobs', domain: 'producthunt.com', aliases: ['product hunt'] },
  { name: 'SaaS Jobs', domain: 'saasjobs.co' },
  { name: 'Startup Jobs', domain: 'startupjobs.com' },
  { name: 'VentureLoop', domain: 'ventureloop.com' },
  { name: 'F6S', domain: 'f6s.com' },
  { name: 'Working in Startups', domain: 'workinstartups.com' },
  { name: 'Loom Jobs', domain: 'loomsdk.com' },
  { name: 'NoDesk', domain: 'nodesk.co' },
  { name: 'Relocate.me', domain: 'relocate.me' },
  { name: 'European Tech Jobs', domain: 'europeantechjobs.com' },
  { name: 'TechCareers', domain: 'techcareers.com' },
  { name: 'ITJobPro', domain: 'itjobpro.com' },
  { name: 'Clevertech', domain: 'clevertech.biz' },
  { name: 'Giganet', domain: 'giganet.work' },

  // ── Remote ──
  { name: 'We Work Remotely', domain: 'weworkremotely.com', aliases: ['wwr'] },
  { name: 'Remote.co', domain: 'remote.co' },
  { name: 'RemoteOK', domain: 'remoteok.com', aliases: ['remote ok'] },
  { name: 'FlexJobs', domain: 'flexjobs.com' },
  { name: 'Working Nomads', domain: 'workingnomads.com' },
  { name: 'Remotive', domain: 'remotive.com' },
  { name: 'JustRemote', domain: 'justremote.co' },
  { name: 'Nodesk', domain: 'nodesk.co' },
  { name: 'Pangian', domain: 'pangian.com' },
  { name: 'Lemon.io', domain: 'lemon.io' },
  { name: 'Remote Leaf', domain: 'remoteleaf.com' },
  { name: 'DailyRemote', domain: 'dailyremote.com' },
  { name: 'Himalayas', domain: 'himalayas.app' },
  { name: 'Contra', domain: 'contra.com' },

  // ── Ireland ──
  { name: 'Jobs.ie', domain: 'jobs.ie' },
  { name: 'IrishJobs.ie', domain: 'irishjobs.ie', aliases: ['irish jobs'] },
  { name: 'CPL', domain: 'cpl.com' },
  { name: 'Hays Ireland', domain: 'hays.ie', aliases: ['hays'] },
  { name: 'Morgan McKinley', domain: 'morganmckinley.com' },
  { name: 'Sigmar Recruitment', domain: 'sigmarrecruitment.com', aliases: ['sigmar'] },
  { name: 'Recruit Ireland', domain: 'recruitireland.com' },
  { name: 'PublicJobs.ie', domain: 'publicjobs.ie', aliases: ['public jobs'] },
  { name: 'GradIreland', domain: 'gradireland.com', aliases: ['grad ireland'] },
  { name: 'Prosperity', domain: 'prosperity.ie' },
  { name: 'Excel Recruitment', domain: 'excelrecruitment.com' },
  { name: 'ComputerJobs.ie', domain: 'computerjobs.ie', aliases: ['computer jobs'] },
  { name: 'NIJobs', domain: 'nijobs.com', aliases: ['northern ireland jobs'] },
  { name: 'ActiveLink', domain: 'activelink.ie' },
  { name: 'FRS Recruitment', domain: 'frsrecruitment.com', aliases: ['frs'] },
  { name: 'Collins McNicholas', domain: 'collinsmcnicholas.ie' },
  { name: 'Berkley Group', domain: 'berkleygroup.com' },
  { name: 'Robert Walters Ireland', domain: 'robertwalters.ie', aliases: ['robert walters'] },
  { name: 'Michael Page Ireland', domain: 'michaelpage.ie', aliases: ['michael page'] },
  { name: 'Reperio', domain: 'reperio.ie' },
  { name: 'Eolas Recruitment', domain: 'eolasrecruitment.ie', aliases: ['eolas'] },
  { name: 'Tech/Life Ireland', domain: 'techlifeireland.com' },
  { name: 'Silicon Republic', domain: 'siliconrepublic.com', aliases: ['silicon republic jobs'] },
  { name: 'Brightwater', domain: 'brightwater.ie' },
  { name: 'Allen Recruitment', domain: 'allenrecruitment.com', aliases: ['allen'] },
  { name: 'Matrix Recruitment', domain: 'matrixrecruitment.ie', aliases: ['matrix'] },
  { name: 'TTM Healthcare', domain: 'ttmhealthcare.com', aliases: ['ttm'] },
  { name: 'Osborne Recruitment', domain: 'osborne.ie', aliases: ['osborne'] },
  { name: 'Cregg Recruitment', domain: 'cregg.ie', aliases: ['cregg'] },
  { name: 'Clark Recruitment', domain: 'clarkrecruitment.ie', aliases: ['clark'] },
  { name: 'Lincoln Recruitment', domain: 'lincolnrecruitment.com', aliases: ['lincoln'] },
  { name: 'Grafton Recruitment', domain: 'graftonrecruitment.com', aliases: ['grafton'] },
  { name: 'ICE Group', domain: 'icegroup.ie', aliases: ['ice'] },
  { name: 'Next Generation', domain: 'nextgeneration.ie' },
  { name: 'Careerwise', domain: 'careerwise.ie' },
  { name: 'Jobbio', domain: 'jobbio.com' },
  { name: 'Abrivia', domain: 'abrivia.ie' },
  { name: 'Lex Consultancy', domain: 'lexconsultancy.ie', aliases: ['lex'] },
  { name: 'Archer Recruitment', domain: 'archerrecruitment.ie', aliases: ['archer'] },
  { name: 'Computer Futures', domain: 'computerfutures.com' },
  { name: 'Harvey Nash', domain: 'harveynash.com' },
  { name: 'Wallace Myers', domain: 'wallacemyers.ie' },
  { name: 'Manpower Ireland', domain: 'manpower.ie' },
  { name: 'Adecco Ireland', domain: 'adecco.ie' },
  { name: 'Randstad Ireland', domain: 'randstad.ie' },
  { name: 'La Crème', domain: 'lacreme.ie', aliases: ['la creme'] },
  { name: 'Eden Recruitment', domain: 'edenrecruitment.ie', aliases: ['eden'] },
  { name: 'Ergo Recruitment', domain: 'ergo.ie', aliases: ['ergo'] },
  { name: 'Kate Cowhig (KFR)', domain: 'kfrrecruitment.com', aliases: ['kate cowhig', 'kfr'] },
  { name: 'Three Q Perms & Temps', domain: '3qrecruitment.ie', aliases: ['3q', 'three q'] },
  { name: 'Fastnet / The Fit Factor', domain: 'fastnet-thefitfactor.ie', aliases: ['fastnet'] },
  { name: 'JobsIreland.ie', domain: 'jobsireland.ie', aliases: ['deasp', 'intreo'] },
  { name: 'Science Foundation Ireland', domain: 'sfi.ie', aliases: ['sfi'] },
  { name: 'IDA Ireland', domain: 'idaireland.com', aliases: ['ida'] },
  { name: 'Enterprise Ireland', domain: 'enterprise-ireland.com' },
  { name: 'TechIreland', domain: 'techireland.org' },

  // ── France ──
  { name: 'Welcome to the Jungle', domain: 'welcometothejungle.com', aliases: ['wttj'] },
  { name: 'HelloWork', domain: 'hellowork.com' },
  { name: 'Cadremploi', domain: 'cadremploi.fr' },
  { name: 'APEC', domain: 'apec.fr' },
  { name: 'France Travail', domain: 'francetravail.fr', aliases: ['pole emploi', 'pôle emploi'] },
  { name: 'LesJeudis', domain: 'lesjeudis.com', aliases: ['les jeudis'] },
  { name: 'Talent.io', domain: 'talent.io' },
  { name: 'Free-Work', domain: 'free-work.com', aliases: ['freelance-info', 'freework'] },
  { name: 'Malt', domain: 'malt.fr' },
  { name: 'Jobteaser', domain: 'jobteaser.com' },
  { name: 'RegionsJob', domain: 'regionsjob.com', aliases: ['regions job'] },
  { name: 'Keljob', domain: 'keljob.com' },
  { name: 'Meteojob', domain: 'meteojob.com' },
  { name: 'ChooseMyCompany', domain: 'choosemycompany.com' },
  { name: 'Wizbii', domain: 'wizbii.com' },
  { name: 'Dogfinance', domain: 'dogfinance.com' },
  { name: 'CIDJ', domain: 'cidj.com' },
  { name: 'Manpower France', domain: 'manpower.fr', aliases: ['manpower'] },
  { name: 'Randstad France', domain: 'randstad.fr', aliases: ['randstad'] },
  { name: 'Adecco France', domain: 'adecco.fr', aliases: ['adecco'] },
  { name: 'Michael Page France', domain: 'michaelpage.fr' },
  { name: 'Robert Half France', domain: 'roberthalf.fr', aliases: ['robert half'] },
  { name: 'Hays France', domain: 'hays.fr' },
  { name: 'PageGroup', domain: 'page.com' },

  // ── UK ──
  { name: 'Reed', domain: 'reed.co.uk' },
  { name: 'Totaljobs', domain: 'totaljobs.com' },
  { name: 'CV-Library', domain: 'cv-library.co.uk', aliases: ['cv library'] },
  { name: 'CWJobs', domain: 'cwjobs.co.uk' },
  { name: 'Jobsite', domain: 'jobsite.co.uk' },
  { name: 'Guardian Jobs', domain: 'jobs.theguardian.com', aliases: ['the guardian'] },
  { name: 'Technojobs', domain: 'technojobs.co.uk' },
  { name: 'IT Jobs Watch', domain: 'itjobswatch.co.uk' },

  // ── Germany / DACH ──
  { name: 'StepStone', domain: 'stepstone.de' },
  { name: 'Xing', domain: 'xing.com' },
  { name: 'Arbeitsagentur', domain: 'arbeitsagentur.de' },
  { name: 'SwissDevJobs', domain: 'swissdevjobs.ch' },
  { name: 'Berlin Startup Jobs', domain: 'berlinstartupjobs.com' },
  { name: 'Startup.jobs', domain: 'startup.jobs' },

  // ── Nordics ──
  { name: 'The Hub', domain: 'thehub.io' },
  { name: 'Jobindex', domain: 'jobindex.dk' },
  { name: 'Finn.no', domain: 'finn.no' },
  { name: 'Arbetsförmedlingen', domain: 'arbetsformedlingen.se' },

  // ── Netherlands / Belgium ──
  { name: 'Nationale Vacaturebank', domain: 'nationalevacaturebank.nl' },
  { name: 'Intermediair', domain: 'intermediair.nl' },
  { name: 'StepStone Belgium', domain: 'stepstone.be' },
  { name: 'VDAB', domain: 'vdab.be' },
  { name: 'Le Forem', domain: 'leforem.be' },

  // ── Spain / Portugal ──
  { name: 'InfoJobs', domain: 'infojobs.net' },
  { name: 'Domestika Jobs', domain: 'domestika.org' },
  { name: 'Landing.jobs', domain: 'landing.jobs' },
  { name: 'Net-Empregos', domain: 'net-empregos.com' },

  // ── Europe-wide ──
  { name: 'EURES', domain: 'eures.europa.eu' },
  { name: 'EuroJobs', domain: 'eurojobs.com' },
  { name: 'EuroTechJobs', domain: 'eurotechjobs.com' },
  { name: 'EU Careers (EPSO)', domain: 'epso.europa.eu', aliases: ['epso', 'eu careers'] },
  { name: 'Euractiv Jobs', domain: 'jobs.euractiv.com' },

  // ── Canada / Australia / International ──
  { name: 'Seek', domain: 'seek.com.au', aliases: ['seek australia'] },
  { name: 'Workopolis', domain: 'workopolis.com' },
  { name: 'Jobillico', domain: 'jobillico.com' },
  { name: 'Naukri', domain: 'naukri.com' },
  { name: 'Bayt', domain: 'bayt.com' },
  { name: 'Wuzzuf', domain: 'wuzzuf.net' },

  // ── Freelance / Gig ──
  { name: 'Upwork', domain: 'upwork.com' },
  { name: 'Fiverr', domain: 'fiverr.com' },
  { name: 'Freelancer', domain: 'freelancer.com' },
  { name: 'Toptal', domain: 'toptal.com' },
  { name: '99designs', domain: '99designs.com' },
  { name: 'PeoplePerHour', domain: 'peopleperhour.com' },
  { name: 'Guru', domain: 'guru.com' },
  { name: 'Codementor', domain: 'codementor.io' },
  { name: 'CloudPeeps', domain: 'cloudpeeps.com' },

  // ── Web3 / Crypto ──
  { name: 'CryptoJobsList', domain: 'cryptojobslist.com' },
  { name: 'Web3.career', domain: 'web3.career' },
  { name: 'Crypto.jobs', domain: 'crypto.jobs' },

  // ── Diversity / Inclusion ──
  { name: 'Diversify Tech', domain: 'diversifytech.com' },
  { name: 'People of Color in Tech', domain: 'pocitjobs.com' },
  { name: 'PowerToFly', domain: 'powertofly.com' },
  { name: 'Tech Ladies', domain: 'hiretechladies.com' },
  { name: 'Elpha', domain: 'elpha.com' },
  { name: 'Jopwell', domain: 'jopwell.com' },

  // ── Staffing agencies (global) ──
  { name: 'Randstad', domain: 'randstad.com' },
  { name: 'Manpower', domain: 'manpower.com' },
  { name: 'Adecco', domain: 'adecco.com' },
  { name: 'Robert Half', domain: 'roberthalf.com' },
  { name: 'Hays', domain: 'hays.com' },
  { name: 'Michael Page', domain: 'michaelpage.com' },
  { name: 'Page Personnel', domain: 'pagepersonnel.com' },
  { name: 'Robert Walters', domain: 'robertwalters.com' },
  { name: 'Spring Professional', domain: 'springprofessional.com' },
  { name: 'Kelly Services', domain: 'kellyservices.com' },

  // ── Non-platform sources ──
  { name: 'Referral', aliases: ['cooptation', 'recommandation', 'referral'] },
  { name: 'Company website', aliases: ['site entreprise', 'career page', 'page carrière', 'careers'] },
  { name: 'Career fair', aliases: ['salon', 'forum emploi', 'job fair'] },
  { name: 'Networking', aliases: ['réseau', 'event', 'meetup'] },
  { name: 'Cold application', aliases: ['candidature spontanée', 'spontaneous', 'unsolicited'] },
  { name: 'Recruiter', aliases: ['recruteur', 'headhunter', 'chasseur de tête'] },
  { name: 'Twitter / X', domain: 'x.com', aliases: ['twitter'] },
  { name: 'Facebook', domain: 'facebook.com' },
  { name: 'Reddit', domain: 'reddit.com' },
]

// ── Shared helpers ──

const _domainIndex = new Map<string, string>()
for (const jb of JOB_BOARDS) {
  if (jb.domain) _domainIndex.set(jb.name.toLowerCase(), jb.domain)
}

export function faviconUrl(domain: string, size = 32): string {
  return `https://www.google.com/s2/favicons?domain=${domain}&sz=${size}`
}

export function getSourceDomain(sourceName: string): string | undefined {
  return _domainIndex.get(sourceName.toLowerCase())
}

export function domainFromUrl(url: string): string | null {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return null
  }
}
