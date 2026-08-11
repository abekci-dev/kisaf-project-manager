/*
 * kisaf — interface strings.
 *
 * One flat object per language, keyed by dotted names. Flat beats nested here:
 * a missing key is a visible `?key` in the UI rather than a crash on an
 * undefined branch, and diffing two languages side by side stays trivial.
 *
 * `app.name` and `app.tagline` are the product's proper name. They are the
 * same in every language and must not be translated.
 *
 * Adding a language:
 *   1. copy the `en` block, translate the values, keep the keys
 *   2. add it to LANGUAGES below
 * Server error text is translated through the `err.*` keys, which match the
 * codes in internal/apperr.
 */
'use strict';

const LANGUAGES = { en: 'English', tr: 'Türkçe' };

const STRINGS = {
  en: {
    'app.name': 'kisaf',
    'app.tagline': 'keep it simple as fuck project manager',

    // Top bar
    'search.placeholder': 'Search projects, tags, tasks or paths…',
    'action.addProject': 'Add Project',
    'action.scanFolder': 'Scan Folder',
    'action.settings': 'Settings',
    'action.close': 'Close',
    'action.cancel': 'Cancel',
    'action.add': 'Add',
    'action.save': 'Save',
    'action.back': '← All projects',

    // Filters
    'filter.all': 'All',
    'filter.favorites': '★ Favorites',
    'filter.dirty': '● Uncommitted',
    'filter.openTasks': '◧ Open tasks',
    'filter.archived': 'Archived',
    'view.grid': 'Card view',
    'view.list': 'Compact list',
    'list.count': '{0} / {1} projects',
    'list.noMatch': 'No matching projects.',

    // Cards
    'card.editor': 'Editor',
    'card.explorer': 'Reveal',
    'card.terminal': 'Terminal',
    'card.favorite': 'Add to / remove from favorites',
    'card.tasks': '{0}/{1} tasks',
    'card.tasksDone': '{0} of {1} tasks done',
    'card.urgent': '{0} urgent',
    'card.noGit': 'no git',
    'card.clean': 'clean',
    'card.archived': 'archived',

    // Empty state
    'empty.title': 'No projects yet',
    'empty.body': 'Use <b>Add Project</b> for one folder, or <b>Scan Folder</b> to find every repository under a directory.',

    // Detail
    'detail.favorite': 'Favorite',
    'detail.openInEditor': 'Open in Editor',
    'detail.editorFor': 'Editor for this project',
    'detail.noEditor': 'No editor found',
    'detail.reveal': 'Reveal in File Manager',
    'detail.openFolder': 'Open Folder',
    'detail.terminal': 'Terminal',
    'detail.copyPath': 'Copy path',
    'detail.pathCopied': 'Path copied',
    'detail.copyFailed': 'Could not copy (no browser permission)',

    'tab.tasks': 'Tasks',
    'tab.overview': 'Overview',
    'tab.git': 'Git',
    'tab.readme': 'README',
    'tab.files': 'Files',

    // Tasks
    'todo.placeholder': 'New task… (e.g. finish the checkout screen)',
    'todo.priority': 'Priority',
    'todo.add': 'Add',
    'todo.filterOpen': 'Open ({0})',
    'todo.filterDone': 'Done ({0})',
    'todo.filterAll': 'All ({0})',
    'todo.none': 'No tasks yet. Add one above — that is how "where did I leave off?" gets answered.',
    'todo.noneInFilter': 'No tasks in this filter.',
    'todo.progress': '{0}/{1} done',
    'todo.clearDone': 'Clear completed',
    'todo.cleared': 'Completed tasks cleared',
    'todo.markDone': 'Mark as done',
    'todo.edit': 'Click to edit',
    'todo.changePriority': 'Change priority',
    'todo.moveUp': 'Move up',
    'todo.moveDown': 'Move down',
    'todo.delete': 'Delete',
    'priority.high': 'High',
    'priority.normal': 'Normal',
    'priority.low': 'Low',

    // Overview
    'overview.description': 'Description',
    'overview.descriptionPlaceholder': 'What is this project about?',
    'overview.notes': 'Notes',
    'overview.notesPlaceholder': 'Free-form notes: decisions, links, reminders…',
    'overview.tags': 'Tags',
    'overview.tagsPlaceholder': 'work, web, archive (comma separated)',
    'overview.info': 'Info',
    'overview.added': 'Added',
    'overview.lastOpened': 'Last opened',
    'overview.openCount': 'Times opened',
    'overview.size': 'Size',
    'overview.calculate': 'Calculate',
    'overview.sizeResult': '{0} · {1} files',
    'overview.sizePartial': '{0} · {1} files (partial)',
    'overview.dangerZone': 'Danger zone',
    'overview.archive': 'Archive',
    'overview.unarchive': 'Unarchive',
    'overview.remove': 'Remove from list',
    'overview.removeNote': 'Removing only deletes the entry from this list. The folder on disk is never touched.',
    'overview.confirmDelete': 'Remove "{0}" from the list?\n\nThe folder on disk will not be deleted.',
    'overview.removed': 'Project removed from the list',

    // Git
    'git.refresh': 'Refresh',
    'git.workingTree': 'Working tree',
    'git.history': 'Commit history',
    'git.noCommits': 'No commits yet.',
    'git.notARepo': 'This folder is not a git repository.',
    'git.failed': 'Could not read git information: {0}',
    'git.ahead': '↑ {0} to push',
    'git.behind': '↓ {0} to pull',
    'git.changes': '{0} changes',
    'git.cleanTree': 'working tree clean',

    // README / files
    'readme.none': 'This project has no README file.',
    'readme.truncated': '(file too long, truncated)',
    'files.empty': 'empty',

    // Add dialog
    'add.title': 'Add Project',
    'add.selectedFolder': 'Selected folder',
    'add.folderPlaceholder': 'e.g. D:\\Projects\\site',
    'add.name': 'Project name',
    'add.nameHint': '(defaults to the folder name)',
    'add.tags': 'Tags',
    'add.tagsHint': '(comma separated)',
    'add.tagsPlaceholder': 'work, web, archive',
    'add.description': 'Description',
    'add.pickFolder': 'Pick a folder.',
    'add.added': '"{0}" added',

    // Scan dialog
    'scan.title': 'Scan Folder',
    'scan.root': 'Root folder to scan',
    'scan.rootPlaceholder': 'e.g. D:\\Projects',
    'scan.depth': 'Depth',
    'scan.includeNonGit': 'Also show folders that are not git repositories',
    'scan.includeNonGitHint': '(package.json, go.mod…)',
    'scan.run': 'Scan',
    'scan.running': 'Scanning…',
    'scan.import': 'Add Selected',
    'scan.selectAll': 'Select all',
    'scan.found': '{0} folders found · {1} new',
    'scan.canAdd': '{0} new projects can be added',
    'scan.selected': '{0} projects selected',
    'scan.nothing': 'No projects found. Try increasing the depth.',
    'scan.pickRoot': 'Pick a root folder first.',
    'scan.alreadyAdded': 'already added',
    'scan.imported': '{0} projects added',
    'scan.importedSkipped': '{0} projects added, {1} skipped',

    // Folder picker
    'picker.parent': '↑ parent folder',
    'picker.noSubfolders': 'No subfolders here. The path shown above is the one that will be used.',

    // Settings
    'settings.title': 'Settings',
    'settings.editor': 'Editor',
    'settings.defaultEditor': 'Default editor',
    'settings.autoFirst': '(automatic: first editor found)',
    'settings.rescan': 'Rescan editors',
    'settings.editorsFound': '{0} editors found',
    'settings.detectedEditors': 'Detected editors',
    'settings.customEditors': 'Add an editor manually',
    'settings.customHint': 'For a program we could not detect, enter its full path. <code>{path}</code> is replaced with the project folder.',
    'settings.addRow': '+ Add row',
    'settings.editorName': 'Name',
    'settings.editorExec': 'Executable',
    'settings.appearance': 'Appearance and behaviour',
    'settings.language': 'Language',
    'settings.languageAuto': 'Same as browser',
    'settings.theme': 'Theme',
    'settings.themeDark': 'Dark',
    'settings.themeLight': 'Light',
    'settings.themeSystem': 'Same as system',
    'settings.listView': 'List view',
    'settings.sort': 'Sort by',
    'settings.sortRecent': 'Recently used',
    'settings.sortName': 'Name',
    'settings.sortCreated': 'Date added',
    'settings.commitCount': 'Commits to show',
    'settings.scanDepth': 'Default scan depth',
    'settings.addresses': 'Access addresses',
    'settings.thisComputer': 'This computer',
    'settings.viaMdns': 'On the LAN (mDNS)',
    'settings.viaIp': 'By IP',
    'settings.remoteAccess': 'Remote access',
    'settings.remoteOn': 'enabled (key required)',
    'settings.remoteOff': 'disabled (this computer only)',
    'settings.remoteHint': 'To enable remote access, set a passphrase in the <code>token</code> field of <code>{0}</code> and restart the app.',
    'settings.files': 'Files',
    'settings.dataFile': 'Data',
    'settings.configFile': 'Config',
    'settings.version': 'Version',
    'settings.gitFound': 'found',
    'settings.gitMissing': 'NOT FOUND — commit history cannot be shown',
    'settings.saved': 'Settings saved',
    'settings.none': 'none',

    // Misc
    'toast.openingEditor': 'Opening "{0}" in your editor…',
    'toast.connectFailed': 'Could not reach the server: {0}',

    // Server error codes (internal/apperr)
    'err.project.notFound': 'Project not found',
    'err.project.duplicate': 'This folder is already tracked: {0}',
    'err.project.notDir': 'That path is not a folder',
    'err.project.folderUnreadable': 'Could not open the folder: {0}',
    'err.path.empty': 'Folder path cannot be empty',
    'err.path.invalid': 'Invalid path',
    'err.path.outside': 'Cannot go outside the project folder',
    'err.todo.notFound': 'Task not found',
    'err.todo.empty': 'Task text cannot be empty',
    'err.todo.limit': 'A project can hold at most {0} tasks',
    'err.editor.notFound': 'No editor found. Pick a default editor in settings, or press “rescan”.',
    'err.editor.notChosen': 'No editor selected or found',
    'err.terminal.notFound': 'No terminal found',
    'err.fileManager.notFound': 'No file manager found',
    'err.action.unknown': 'Unknown action: {0}',
    'err.file.notFound': 'File not found',
    'err.file.binary': 'This file is not text (binary content)',
    'err.dir.unreadable': 'Could not read the folder: {0}',
    'err.request.invalid': 'Invalid request: {0}',
    'err.scan.failed': 'Scan failed: {0}',
    'err.auth.remoteDisabled': 'Remote access is disabled.',
    'err.auth.required': 'Sign-in required',
    'err.origin.rejected': 'Request from a different origin was rejected',
  },

  tr: {
    'app.name': 'kisaf',
    'app.tagline': 'keep it simple as fuck project manager',

    'search.placeholder': 'Proje, etiket, görev veya yol ara…',
    'action.addProject': 'Proje Ekle',
    'action.scanFolder': 'Klasör Tara',
    'action.settings': 'Ayarlar',
    'action.close': 'Kapat',
    'action.cancel': 'Vazgeç',
    'action.add': 'Ekle',
    'action.save': 'Kaydet',
    'action.back': '← Tüm projeler',

    'filter.all': 'Tümü',
    'filter.favorites': '★ Favoriler',
    'filter.dirty': '● Değişiklik var',
    'filter.openTasks': '◧ Açık görev',
    'filter.archived': 'Arşiv',
    'view.grid': 'Kart görünümü',
    'view.list': 'Sıkışık liste',
    'list.count': '{0} / {1} proje',
    'list.noMatch': 'Eşleşen proje yok.',

    'card.editor': 'Editör',
    'card.explorer': 'Explorer',
    'card.terminal': 'Terminal',
    'card.favorite': 'Favorilere ekle/çıkar',
    'card.tasks': '{0}/{1} görev',
    'card.tasksDone': '{1} görevin {0} tanesi tamam',
    'card.urgent': '{0} acil',
    'card.noGit': 'git yok',
    'card.clean': 'temiz',
    'card.archived': 'arşivlenmiş',

    'empty.title': 'Henüz proje yok',
    'empty.body': 'Tek tek eklemek için <b>Proje Ekle</b>, bir klasördeki tüm depoları bulmak için <b>Klasör Tara</b>.',

    'detail.favorite': 'Favori',
    'detail.openInEditor': 'Editörde Aç',
    'detail.editorFor': 'Bu proje için editör',
    'detail.noEditor': 'Editör bulunamadı',
    'detail.reveal': "Explorer'da Göster",
    'detail.openFolder': 'Klasörü Aç',
    'detail.terminal': 'Terminal',
    'detail.copyPath': 'Yolu kopyala',
    'detail.pathCopied': 'Yol kopyalandı',
    'detail.copyFailed': 'Kopyalanamadı (tarayıcı izni yok)',

    'tab.tasks': 'Görevler',
    'tab.overview': 'Genel',
    'tab.git': 'Git',
    'tab.readme': 'README',
    'tab.files': 'Dosyalar',

    'todo.placeholder': 'Yeni görev… (örn. ödeme ekranını bitir)',
    'todo.priority': 'Öncelik',
    'todo.add': 'Ekle',
    'todo.filterOpen': 'Açık ({0})',
    'todo.filterDone': 'Tamamlanan ({0})',
    'todo.filterAll': 'Tümü ({0})',
    'todo.none': 'Henüz görev yok. Yukarıdan ekleyin — "nerede kalmıştım" sorusu böyle çözülüyor.',
    'todo.noneInFilter': 'Bu filtrede görev yok.',
    'todo.progress': '{0}/{1} tamamlandı',
    'todo.clearDone': 'Tamamlananları temizle',
    'todo.cleared': 'Tamamlanan görevler temizlendi',
    'todo.markDone': 'Tamamlandı olarak işaretle',
    'todo.edit': 'Düzenlemek için tıklayın',
    'todo.changePriority': 'Önceliği değiştir',
    'todo.moveUp': 'Yukarı taşı',
    'todo.moveDown': 'Aşağı taşı',
    'todo.delete': 'Sil',
    'priority.high': 'Yüksek',
    'priority.normal': 'Normal',
    'priority.low': 'Düşük',

    'overview.description': 'Açıklama',
    'overview.descriptionPlaceholder': 'Bu proje neyle ilgili?',
    'overview.notes': 'Notlar',
    'overview.notesPlaceholder': 'Serbest not: kararlar, bağlantılar, hatırlatmalar…',
    'overview.tags': 'Etiketler',
    'overview.tagsPlaceholder': 'iş, web, arşiv (virgülle ayırın)',
    'overview.info': 'Bilgi',
    'overview.added': 'Eklendi',
    'overview.lastOpened': 'Son açılış',
    'overview.openCount': 'Açılma sayısı',
    'overview.size': 'Boyut',
    'overview.calculate': 'Hesapla',
    'overview.sizeResult': '{0} · {1} dosya',
    'overview.sizePartial': '{0} · {1} dosya (kısmi)',
    'overview.dangerZone': 'Tehlikeli bölge',
    'overview.archive': 'Arşivle',
    'overview.unarchive': 'Arşivden çıkar',
    'overview.remove': 'Listeden kaldır',
    'overview.removeNote': 'Kaldırmak yalnızca bu listeden siler. Diskteki klasöre hiçbir zaman dokunulmaz.',
    'overview.confirmDelete': '"{0}" listeden kaldırılsın mı?\n\nDiskteki klasör silinmez.',
    'overview.removed': 'Proje listeden kaldırıldı',

    'git.refresh': 'Yenile',
    'git.workingTree': 'Çalışma alanı',
    'git.history': 'Commit geçmişi',
    'git.noCommits': 'Henüz commit yok.',
    'git.notARepo': 'Bu klasör bir git deposu değil.',
    'git.failed': 'Git bilgisi alınamadı: {0}',
    'git.ahead': '↑ {0} gönderilmemiş',
    'git.behind': '↓ {0} alınmamış',
    'git.changes': '{0} değişiklik',
    'git.cleanTree': 'çalışma alanı temiz',

    'readme.none': 'Bu projede README dosyası yok.',
    'readme.truncated': '(dosya çok uzun, kırpıldı)',
    'files.empty': 'boş',

    'add.title': 'Proje Ekle',
    'add.selectedFolder': 'Seçili klasör',
    'add.folderPlaceholder': 'Örn. D:\\Projeler\\site',
    'add.name': 'Proje adı',
    'add.nameHint': '(boş bırakılırsa klasör adı)',
    'add.tags': 'Etiketler',
    'add.tagsHint': '(virgülle)',
    'add.tagsPlaceholder': 'iş, web, arşiv',
    'add.description': 'Açıklama',
    'add.pickFolder': 'Bir klasör seçin.',
    'add.added': '"{0}" eklendi',

    'scan.title': 'Klasör Tara',
    'scan.root': 'Taranacak kök klasör',
    'scan.rootPlaceholder': 'Örn. D:\\Projeler',
    'scan.depth': 'Derinlik',
    'scan.includeNonGit': 'Git deposu olmayanları da göster',
    'scan.includeNonGitHint': '(package.json, go.mod…)',
    'scan.run': 'Tara',
    'scan.running': 'Taranıyor…',
    'scan.import': 'Seçilenleri Ekle',
    'scan.selectAll': 'Tümünü seç',
    'scan.found': '{0} klasör bulundu · {1} yeni',
    'scan.canAdd': '{0} yeni proje eklenebilir',
    'scan.selected': '{0} proje seçildi',
    'scan.nothing': 'Hiçbir proje bulunamadı. Derinliği artırmayı deneyin.',
    'scan.pickRoot': 'Önce bir kök klasör seçin.',
    'scan.alreadyAdded': 'zaten ekli',
    'scan.imported': '{0} proje eklendi',
    'scan.importedSkipped': '{0} proje eklendi, {1} atlandı',

    'picker.parent': '↑ üst klasör',
    'picker.noSubfolders': 'Bu klasörde alt klasör yok. Yukarıdaki kutuda görünen yol seçilidir.',

    'settings.title': 'Ayarlar',
    'settings.editor': 'Editör',
    'settings.defaultEditor': 'Varsayılan editör',
    'settings.autoFirst': '(otomatik: bulunan ilk editör)',
    'settings.rescan': 'Editörleri yeniden tara',
    'settings.editorsFound': '{0} editör bulundu',
    'settings.detectedEditors': 'Bulunan editörler',
    'settings.customEditors': 'Elle editör ekle',
    'settings.customHint': 'Otomatik bulunamayan bir program için tam yolu yazın. <code>{path}</code> proje klasörüyle değiştirilir.',
    'settings.addRow': '+ Satır ekle',
    'settings.editorName': 'Ad',
    'settings.editorExec': 'Çalıştırılacak dosya',
    'settings.appearance': 'Görünüm ve davranış',
    'settings.language': 'Dil',
    'settings.languageAuto': 'Tarayıcıyla aynı',
    'settings.theme': 'Tema',
    'settings.themeDark': 'Koyu',
    'settings.themeLight': 'Açık',
    'settings.themeSystem': 'Sistemle aynı',
    'settings.listView': 'Liste görünümü',
    'settings.sort': 'Sıralama',
    'settings.sortRecent': 'Son kullanılan',
    'settings.sortName': 'İsim',
    'settings.sortCreated': 'Eklenme tarihi',
    'settings.commitCount': 'Gösterilecek commit sayısı',
    'settings.scanDepth': 'Varsayılan tarama derinliği',
    'settings.addresses': 'Erişim adresleri',
    'settings.thisComputer': 'Bu bilgisayar',
    'settings.viaMdns': 'Ağdan (mDNS)',
    'settings.viaIp': 'IP ile',
    'settings.remoteAccess': 'Uzaktan erişim',
    'settings.remoteOn': 'açık (anahtar gerekli)',
    'settings.remoteOff': 'kapalı (yalnızca bu bilgisayar)',
    'settings.remoteHint': 'Uzaktan erişimi açmak için <code>{0}</code> dosyasındaki <code>token</code> alanına bir parola yazıp uygulamayı yeniden başlatın.',
    'settings.files': 'Dosyalar',
    'settings.dataFile': 'Veri',
    'settings.configFile': 'Ayarlar',
    'settings.version': 'Sürüm',
    'settings.gitFound': 'bulundu',
    'settings.gitMissing': 'BULUNAMADI — commit geçmişi gösterilemez',
    'settings.saved': 'Ayarlar kaydedildi',
    'settings.none': 'yok',

    'toast.openingEditor': '"{0}" editörde açılıyor…',
    'toast.connectFailed': 'Sunucuya bağlanılamadı: {0}',

    'err.project.notFound': 'Proje bulunamadı',
    'err.project.duplicate': 'Bu klasör zaten ekli: {0}',
    'err.project.notDir': 'Verilen yol bir klasör değil',
    'err.project.folderUnreadable': 'Klasör açılamadı: {0}',
    'err.path.empty': 'Klasör yolu boş olamaz',
    'err.path.invalid': 'Geçersiz yol',
    'err.path.outside': 'Proje klasörünün dışına çıkılamaz',
    'err.todo.notFound': 'Görev bulunamadı',
    'err.todo.empty': 'Görev metni boş olamaz',
    'err.todo.limit': 'Bir projede en fazla {0} görev olabilir',
    'err.editor.notFound': 'Editör bulunamadı. Ayarlardan varsayılan editörü seçin veya "yeniden tara" deyin.',
    'err.editor.notChosen': 'Editör seçilmedi veya bulunamadı',
    'err.terminal.notFound': 'Terminal bulunamadı',
    'err.fileManager.notFound': 'Dosya yöneticisi bulunamadı',
    'err.action.unknown': 'Bilinmeyen işlem: {0}',
    'err.file.notFound': 'Dosya bulunamadı',
    'err.file.binary': 'Bu dosya metin değil (ikili içerik)',
    'err.dir.unreadable': 'Klasör okunamadı: {0}',
    'err.request.invalid': 'Geçersiz istek: {0}',
    'err.scan.failed': 'Tarama başarısız: {0}',
    'err.auth.remoteDisabled': 'Uzaktan erişim kapalı.',
    'err.auth.required': 'Oturum gerekli',
    'err.origin.rejected': 'Farklı bir kaynaktan gelen istek reddedildi',
  },
};

/** Currently selected language code; set by applyLanguage(). */
let currentLang = 'en';

/**
 * resolveLanguage turns a stored preference into a language we actually have.
 * "auto" follows the browser, and an unknown browser language lands on English
 * rather than showing raw keys.
 */
function resolveLanguage(preference) {
  if (preference && preference !== 'auto' && STRINGS[preference]) return preference;
  for (const tag of navigator.languages || [navigator.language || 'en']) {
    const base = String(tag).toLowerCase().split('-')[0];
    if (STRINGS[base]) return base;
  }
  return 'en';
}

function applyLanguage(preference) {
  currentLang = resolveLanguage(preference);
  document.documentElement.lang = currentLang;
  return currentLang;
}

/**
 * t looks up a key and substitutes {0}, {1}, … positionally.
 * An unknown key renders as "?key" so a gap is obvious in the UI and in tests,
 * instead of silently disappearing.
 */
function t(key, ...args) {
  const table = STRINGS[currentLang] || STRINGS.en;
  let text = table[key];
  if (text === undefined) text = STRINGS.en[key];
  if (text === undefined) return `?${key}`;
  return text.replace(/\{(\d+)\}/g, (whole, i) => {
    const value = args[Number(i)];
    return value === undefined ? whole : String(value);
  });
}

/**
 * tError localises a failure from the API.
 *
 * The server sends English prose plus a stable code and its arguments; if the
 * code is one we know, we rebuild the sentence in the active language, and
 * otherwise we show what the server said rather than nothing.
 */
function tError(err) {
  const code = err && err.code;
  if (code) {
    const key = `err.${code}`;
    const table = STRINGS[currentLang] || STRINGS.en;
    if (table[key] !== undefined || STRINGS.en[key] !== undefined) {
      return t(key, ...(err.args || []));
    }
  }
  return (err && err.message) || String(err);
}

/** Applies translations to static markup carrying data-i18n attributes. */
function translateDocument(root = document) {
  root.querySelectorAll('[data-i18n]').forEach((el) => {
    el.textContent = t(el.dataset.i18n);
  });
  root.querySelectorAll('[data-i18n-html]').forEach((el) => {
    el.innerHTML = t(el.dataset.i18nHtml);
  });
  for (const attr of ['placeholder', 'title', 'aria-label']) {
    root.querySelectorAll(`[data-i18n-${attr}]`).forEach((el) => {
      el.setAttribute(attr, t(el.dataset[`i18n${attr.replace(/(^|-)([a-z])/g, (_, __, c) => c.toUpperCase())}`]));
    });
  }
}
