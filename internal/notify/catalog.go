// Package notify is the discodrive notification system: event catalog, channels,
// branded HTML email layout, and Notifier (see spec 1.4).
package notify

// Category distinguishes mandatory security alerts from optional activity events.
type Category string

const (
	Security Category = "security"
	Activity Category = "activity"
)

// DefaultLang is the server-side default language (until users.locale exists; i18n seam).
const DefaultLang = "en"

// Template is the content block for an event (header/footer are added by the layout in layout.go).
type Template struct {
	Subject string // text/template
	HTML    string // html/template — inner block
	Text    string // text/template — plain-text variant
}

// Event describes a notification type.
type Event struct {
	Key             string
	Category        Category
	Mandatory       bool
	DefaultChannels []string
	Templates       map[string]Template
}

// Catalog holds all events (some defined in spec 1.4, some reserved for future features).
var Catalog = map[string]Event{
	"share.received": {
		Key: "share.received", Category: Activity, Mandatory: false, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "{{.SharerEmail}} shared a {{.ResourceLabel}} with you: «{{.NodeName}}»",
				HTML:    `<h1>New share</h1><p>{{.SharerEmail}} has shared the {{.ResourceLabel}} «<b>{{.NodeName}}</b>» with you.</p><p>Open the "Shared with me" section to view it.</p>`,
				Text:    `{{.SharerEmail}} has shared the {{.ResourceLabel}} «{{.NodeName}}» with you. Open the "Shared with me" section to view it.`,
			},
			"ru": {
				Subject: "Вам открыли доступ к {{.ResourceLabel}} «{{.NodeName}}»",
				HTML:    `<h1>Доступ открыт</h1><p>{{.SharerEmail}} открыл вам доступ к {{.ResourceLabel}} «<b>{{.NodeName}}</b>».</p><p>Откройте раздел «Доступные мне», чтобы посмотреть.</p>`,
				Text:    "{{.SharerEmail}} открыл вам доступ к {{.ResourceLabel}} «{{.NodeName}}». Откройте раздел «Доступные мне».",
			},
			"de": {
				Subject: "{{.SharerEmail}} hat {{.ResourceLabel}} mit Ihnen geteilt: «{{.NodeName}}»",
				HTML:    `<h1>Neue Freigabe</h1><p>{{.SharerEmail}} hat {{.ResourceLabel}} «<b>{{.NodeName}}</b>» mit Ihnen geteilt.</p><p>Öffnen Sie den Bereich „Mit mir geteilt“, um es anzusehen.</p>`,
				Text:    "{{.SharerEmail}} hat {{.ResourceLabel}} «{{.NodeName}}» mit Ihnen geteilt. Öffnen Sie den Bereich „Mit mir geteilt“.",
			},
			"uk": {
				Subject: "{{.SharerEmail}} надав вам доступ до {{.ResourceLabel}} «{{.NodeName}}»",
				HTML:    `<h1>Новий доступ</h1><p>{{.SharerEmail}} надав вам доступ до {{.ResourceLabel}} «<b>{{.NodeName}}</b>».</p><p>Відкрийте розділ «Доступні мені», щоб переглянути.</p>`,
				Text:    "{{.SharerEmail}} надав вам доступ до {{.ResourceLabel}} «{{.NodeName}}». Відкрийте розділ «Доступні мені».",
			},
			"fr": {
				Subject: "{{.SharerEmail}} a partagé {{.ResourceLabel}} avec vous : «{{.NodeName}}»",
				HTML:    `<h1>Nouveau partage</h1><p>{{.SharerEmail}} a partagé {{.ResourceLabel}} «<b>{{.NodeName}}</b>» avec vous.</p><p>Ouvrez la section « Partagés avec moi » pour le consulter.</p>`,
				Text:    "{{.SharerEmail}} a partagé {{.ResourceLabel}} «{{.NodeName}}» avec vous. Ouvrez la section « Partagés avec moi ».",
			},
			"es": {
				Subject: "{{.SharerEmail}} compartió {{.ResourceLabel}} contigo: «{{.NodeName}}»",
				HTML:    `<h1>Nuevo recurso compartido</h1><p>{{.SharerEmail}} compartió {{.ResourceLabel}} «<b>{{.NodeName}}</b>» contigo.</p><p>Abre la sección «Compartido conmigo» para verlo.</p>`,
				Text:    "{{.SharerEmail}} compartió {{.ResourceLabel}} «{{.NodeName}}» contigo. Abre la sección «Compartido conmigo».",
			},
			"sr": {
				Subject: "{{.SharerEmail}} вам је поделио {{.ResourceLabel}}: «{{.NodeName}}»",
				HTML:    `<h1>Ново дељење</h1><p>{{.SharerEmail}} вам је поделио {{.ResourceLabel}} «<b>{{.NodeName}}</b>».</p><p>Отворите одељак „Подељено са мном“ да бисте видели.</p>`,
				Text:    "{{.SharerEmail}} вам је поделио {{.ResourceLabel}} «{{.NodeName}}». Отворите одељак „Подељено са мном“.",
			},
		},
	},
	"device.password_added": {
		Key: "device.password_added", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Device password added: {{.DeviceName}}",
				HTML:    `<h1>New device password</h1><p>A device access password has been created for «<b>{{.DeviceName}}</b>» on your account.</p><p>If this wasn't you, change your password and remove the device in settings.</p>`,
				Text:    "A device access password was created for «{{.DeviceName}}». If this wasn't you, change your password and remove the device.",
			},
			"ru": {
				Subject: "Добавлен пароль устройства: {{.DeviceName}}",
				HTML:    `<h1>Новый пароль устройства</h1><p>Для вашего аккаунта создан пароль доступа устройства «<b>{{.DeviceName}}</b>».</p><p>Если это были не вы — смените пароль и отключите устройство в настройках.</p>`,
				Text:    "Создан пароль устройства «{{.DeviceName}}». Если это были не вы — смените пароль и отключите устройство.",
			},
			"de": {
				Subject: "Gerätepasswort hinzugefügt: {{.DeviceName}}",
				HTML:    `<h1>Neues Gerätepasswort</h1><p>Für Ihr Konto wurde ein Gerätezugangspasswort für «<b>{{.DeviceName}}</b>» erstellt.</p><p>Falls Sie das nicht waren, ändern Sie Ihr Passwort und entfernen Sie das Gerät in den Einstellungen.</p>`,
				Text:    "Ein Gerätepasswort für «{{.DeviceName}}» wurde erstellt. Falls Sie das nicht waren, ändern Sie Ihr Passwort und entfernen Sie das Gerät.",
			},
			"uk": {
				Subject: "Додано пароль пристрою: {{.DeviceName}}",
				HTML:    `<h1>Новий пароль пристрою</h1><p>Для вашого облікового запису створено пароль доступу пристрою «<b>{{.DeviceName}}</b>».</p><p>Якщо це були не ви — змініть пароль і вилучіть пристрій у налаштуваннях.</p>`,
				Text:    "Створено пароль пристрою «{{.DeviceName}}». Якщо це були не ви — змініть пароль і вилучіть пристрій.",
			},
			"fr": {
				Subject: "Mot de passe d'appareil ajouté : {{.DeviceName}}",
				HTML:    `<h1>Nouveau mot de passe d'appareil</h1><p>Un mot de passe d'accès pour l'appareil «<b>{{.DeviceName}}</b>» a été créé sur votre compte.</p><p>Si ce n'était pas vous, changez votre mot de passe et supprimez l'appareil dans les paramètres.</p>`,
				Text:    "Un mot de passe pour l'appareil «{{.DeviceName}}» a été créé. Si ce n'était pas vous, changez votre mot de passe et supprimez l'appareil.",
			},
			"es": {
				Subject: "Contraseña de dispositivo añadida: {{.DeviceName}}",
				HTML:    `<h1>Nueva contraseña de dispositivo</h1><p>Se ha creado una contraseña de acceso para el dispositivo «<b>{{.DeviceName}}</b>» en tu cuenta.</p><p>Si no fuiste tú, cambia tu contraseña y elimina el dispositivo en los ajustes.</p>`,
				Text:    "Se creó una contraseña para el dispositivo «{{.DeviceName}}». Si no fuiste tú, cambia tu contraseña y elimina el dispositivo.",
			},
			"sr": {
				Subject: "Додата лозинка уређаја: {{.DeviceName}}",
				HTML:    `<h1>Нова лозинка уређаја</h1><p>За ваш налог је креирана лозинка за приступ уређаја «<b>{{.DeviceName}}</b>».</p><p>Ако то нисте били ви — промените лозинку и уклоните уређај у подешавањима.</p>`,
				Text:    "Креирана је лозинка уређаја «{{.DeviceName}}». Ако то нисте били ви — промените лозинку и уклоните уређај.",
			},
		},
	},
	"login.new_device": {
		Key: "login.new_device", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "New device sign-in",
				HTML:    `<h1>New sign-in</h1><p>A sign-in to your account from a new device was detected.</p><p><b>Device:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Time:</b> {{.Time}}</p><p>If this wasn't you, change your password immediately.</p>`,
				Text:    "New device sign-in. Device: {{.UserAgent}}; IP: {{.IP}}; time: {{.Time}}. If this wasn't you, change your password immediately.",
			},
			"ru": {
				Subject: "Вход с нового устройства",
				HTML:    `<h1>Новый вход</h1><p>Зафиксирован вход в ваш аккаунт с нового устройства.</p><p><b>Устройство:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Время:</b> {{.Time}}</p><p>Если это были не вы — немедленно смените пароль.</p>`,
				Text:    "Вход с нового устройства. Устройство: {{.UserAgent}}; IP: {{.IP}}; время: {{.Time}}. Если это были не вы — смените пароль.",
			},
			"de": {
				Subject: "Anmeldung von einem neuen Gerät",
				HTML:    `<h1>Neue Anmeldung</h1><p>Eine Anmeldung bei Ihrem Konto von einem neuen Gerät wurde erkannt.</p><p><b>Gerät:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Zeit:</b> {{.Time}}</p><p>Falls Sie das nicht waren, ändern Sie sofort Ihr Passwort.</p>`,
				Text:    "Anmeldung von einem neuen Gerät. Gerät: {{.UserAgent}}; IP: {{.IP}}; Zeit: {{.Time}}. Falls Sie das nicht waren, ändern Sie Ihr Passwort.",
			},
			"uk": {
				Subject: "Вхід із нового пристрою",
				HTML:    `<h1>Новий вхід</h1><p>Зафіксовано вхід у ваш обліковий запис із нового пристрою.</p><p><b>Пристрій:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Час:</b> {{.Time}}</p><p>Якщо це були не ви — негайно змініть пароль.</p>`,
				Text:    "Вхід із нового пристрою. Пристрій: {{.UserAgent}}; IP: {{.IP}}; час: {{.Time}}. Якщо це були не ви — змініть пароль.",
			},
			"fr": {
				Subject: "Connexion depuis un nouvel appareil",
				HTML:    `<h1>Nouvelle connexion</h1><p>Une connexion à votre compte depuis un nouvel appareil a été détectée.</p><p><b>Appareil :</b> {{.UserAgent}}<br><b>IP :</b> {{.IP}}<br><b>Heure :</b> {{.Time}}</p><p>Si ce n'était pas vous, changez immédiatement votre mot de passe.</p>`,
				Text:    "Connexion depuis un nouvel appareil. Appareil : {{.UserAgent}} ; IP : {{.IP}} ; heure : {{.Time}}. Si ce n'était pas vous, changez votre mot de passe.",
			},
			"es": {
				Subject: "Inicio de sesión desde un nuevo dispositivo",
				HTML:    `<h1>Nuevo inicio de sesión</h1><p>Se ha detectado un inicio de sesión en tu cuenta desde un nuevo dispositivo.</p><p><b>Dispositivo:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Hora:</b> {{.Time}}</p><p>Si no fuiste tú, cambia tu contraseña de inmediato.</p>`,
				Text:    "Inicio de sesión desde un nuevo dispositivo. Dispositivo: {{.UserAgent}}; IP: {{.IP}}; hora: {{.Time}}. Si no fuiste tú, cambia tu contraseña.",
			},
			"sr": {
				Subject: "Пријава са новог уређаја",
				HTML:    `<h1>Нова пријава</h1><p>Забележена је пријава на ваш налог са новог уређаја.</p><p><b>Уређај:</b> {{.UserAgent}}<br><b>IP:</b> {{.IP}}<br><b>Време:</b> {{.Time}}</p><p>Ако то нисте били ви — одмах промените лозинку.</p>`,
				Text:    "Пријава са новог уређаја. Уређај: {{.UserAgent}}; IP: {{.IP}}; време: {{.Time}}. Ако то нисте били ви — промените лозинку.",
			},
		},
	},
	"quota.near_limit": {
		Key: "quota.near_limit", Category: Activity, Mandatory: false, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Storage almost full ({{.Percent}}%)",
				HTML:    `<h1>Running low on space</h1><p>You've used {{.Percent}}% of your storage ({{.Used}} of {{.Quota}}).</p><p>Delete unwanted files or empty the trash to free up space.</p>`,
				Text:    "You've used {{.Percent}}% of your storage ({{.Used}} of {{.Quota}}). Free up some space.",
			},
			"ru": {
				Subject: "Хранилище почти заполнено ({{.Percent}}%)",
				HTML:    `<h1>Заканчивается место</h1><p>Использовано {{.Percent}}% хранилища ({{.Used}} из {{.Quota}}).</p><p>Удалите ненужные файлы или очистите корзину, чтобы освободить место.</p>`,
				Text:    "Использовано {{.Percent}}% хранилища ({{.Used}} из {{.Quota}}). Освободите место.",
			},
			"de": {
				Subject: "Speicher fast voll ({{.Percent}}%)",
				HTML:    `<h1>Wenig Speicherplatz</h1><p>Sie haben {{.Percent}}% Ihres Speichers belegt ({{.Used}} von {{.Quota}}).</p><p>Löschen Sie nicht benötigte Dateien oder leeren Sie den Papierkorb, um Platz freizugeben.</p>`,
				Text:    "Sie haben {{.Percent}}% Ihres Speichers belegt ({{.Used}} von {{.Quota}}). Geben Sie Speicherplatz frei.",
			},
			"uk": {
				Subject: "Сховище майже заповнене ({{.Percent}}%)",
				HTML:    `<h1>Закінчується місце</h1><p>Використано {{.Percent}}% сховища ({{.Used}} з {{.Quota}}).</p><p>Видаліть непотрібні файли або очистіть кошик, щоб звільнити місце.</p>`,
				Text:    "Використано {{.Percent}}% сховища ({{.Used}} з {{.Quota}}). Звільніть місце.",
			},
			"fr": {
				Subject: "Stockage presque plein ({{.Percent}} %)",
				HTML:    `<h1>Espace bientôt épuisé</h1><p>Vous avez utilisé {{.Percent}} % de votre stockage ({{.Used}} sur {{.Quota}}).</p><p>Supprimez les fichiers inutiles ou videz la corbeille pour libérer de l'espace.</p>`,
				Text:    "Vous avez utilisé {{.Percent}} % de votre stockage ({{.Used}} sur {{.Quota}}). Libérez de l'espace.",
			},
			"es": {
				Subject: "Almacenamiento casi lleno ({{.Percent}}%)",
				HTML:    `<h1>Queda poco espacio</h1><p>Has usado el {{.Percent}}% de tu almacenamiento ({{.Used}} de {{.Quota}}).</p><p>Elimina archivos innecesarios o vacía la papelera para liberar espacio.</p>`,
				Text:    "Has usado el {{.Percent}}% de tu almacenamiento ({{.Used}} de {{.Quota}}). Libera espacio.",
			},
			"sr": {
				Subject: "Складиште је скоро пуно ({{.Percent}}%)",
				HTML:    `<h1>Понестаје простора</h1><p>Искористили сте {{.Percent}}% складишта ({{.Used}} од {{.Quota}}).</p><p>Обришите непотребне датотеке или испразните корпу да ослободите простор.</p>`,
				Text:    "Искористили сте {{.Percent}}% складишта ({{.Used}} од {{.Quota}}). Ослободите простор.",
			},
		},
	},
	"account.password_changed": {
		Key: "account.password_changed", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Your password was changed",
				HTML:    `<h1>Password changed</h1><p>Your account password has been changed. If this wasn't you, contact your administrator.</p>`,
				Text:    "Your account password has been changed. If this wasn't you, contact your administrator.",
			},
			"ru": {
				Subject: "Пароль изменён",
				HTML:    `<h1>Пароль изменён</h1><p>Пароль вашего аккаунта был изменён. Если это были не вы — обратитесь к администратору.</p>`,
				Text:    "Пароль вашего аккаунта был изменён. Если это были не вы — обратитесь к администратору.",
			},
			"de": {
				Subject: "Ihr Passwort wurde geändert",
				HTML:    `<h1>Passwort geändert</h1><p>Das Passwort Ihres Kontos wurde geändert. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.</p>`,
				Text:    "Das Passwort Ihres Kontos wurde geändert. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.",
			},
			"uk": {
				Subject: "Пароль змінено",
				HTML:    `<h1>Пароль змінено</h1><p>Пароль вашого облікового запису було змінено. Якщо це були не ви — зверніться до адміністратора.</p>`,
				Text:    "Пароль вашого облікового запису було змінено. Якщо це були не ви — зверніться до адміністратора.",
			},
			"fr": {
				Subject: "Votre mot de passe a été modifié",
				HTML:    `<h1>Mot de passe modifié</h1><p>Le mot de passe de votre compte a été modifié. Si ce n'était pas vous, contactez votre administrateur.</p>`,
				Text:    "Le mot de passe de votre compte a été modifié. Si ce n'était pas vous, contactez votre administrateur.",
			},
			"es": {
				Subject: "Tu contraseña ha sido cambiada",
				HTML:    `<h1>Contraseña cambiada</h1><p>La contraseña de tu cuenta ha sido cambiada. Si no fuiste tú, contacta con tu administrador.</p>`,
				Text:    "La contraseña de tu cuenta ha sido cambiada. Si no fuiste tú, contacta con tu administrador.",
			},
			"sr": {
				Subject: "Ваша лозинка је промењена",
				HTML:    `<h1>Лозинка промењена</h1><p>Лозинка вашег налога је промењена. Ако то нисте били ви — обратите се администратору.</p>`,
				Text:    "Лозинка вашег налога је промењена. Ако то нисте били ви — обратите се администратору.",
			},
		},
	},
	"account.totp_enabled": {
		Key: "account.totp_enabled", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Two-factor authentication enabled",
				HTML:    `<h1>Two-factor authentication enabled</h1><p>Two-factor authentication (TOTP) has been turned on for your account. If this wasn't you, contact your administrator.</p>`,
				Text:    "Two-factor authentication (TOTP) has been turned on for your account. If this wasn't you, contact your administrator.",
			},
			"ru": {
				Subject: "Двухфакторная аутентификация включена",
				HTML:    `<h1>Двухфакторная аутентификация включена</h1><p>Для вашего аккаунта включена двухфакторная аутентификация (TOTP). Если это были не вы — обратитесь к администратору.</p>`,
				Text:    "Для вашего аккаунта включена двухфакторная аутентификация (TOTP). Если это были не вы — обратитесь к администратору.",
			},
			"de": {
				Subject: "Zwei-Faktor-Authentifizierung aktiviert",
				HTML:    `<h1>Zwei-Faktor-Authentifizierung aktiviert</h1><p>Die Zwei-Faktor-Authentifizierung (TOTP) wurde für Ihr Konto aktiviert. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.</p>`,
				Text:    "Die Zwei-Faktor-Authentifizierung (TOTP) wurde für Ihr Konto aktiviert. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.",
			},
			"uk": {
				Subject: "Двофакторну автентифікацію ввімкнено",
				HTML:    `<h1>Двофакторну автентифікацію ввімкнено</h1><p>Для вашого облікового запису ввімкнено двофакторну автентифікацію (TOTP). Якщо це були не ви — зверніться до адміністратора.</p>`,
				Text:    "Для вашого облікового запису ввімкнено двофакторну автентифікацію (TOTP). Якщо це були не ви — зверніться до адміністратора.",
			},
			"fr": {
				Subject: "Authentification à deux facteurs activée",
				HTML:    `<h1>Authentification à deux facteurs activée</h1><p>L'authentification à deux facteurs (TOTP) a été activée pour votre compte. Si ce n'était pas vous, contactez votre administrateur.</p>`,
				Text:    "L'authentification à deux facteurs (TOTP) a été activée pour votre compte. Si ce n'était pas vous, contactez votre administrateur.",
			},
			"es": {
				Subject: "Autenticación de dos factores activada",
				HTML:    `<h1>Autenticación de dos factores activada</h1><p>Se ha activado la autenticación de dos factores (TOTP) en tu cuenta. Si no fuiste tú, contacta con tu administrador.</p>`,
				Text:    "Se ha activado la autenticación de dos factores (TOTP) en tu cuenta. Si no fuiste tú, contacta con tu administrador.",
			},
			"sr": {
				Subject: "Двофакторска аутентификација укључена",
				HTML:    `<h1>Двофакторска аутентификација укључена</h1><p>За ваш налог је укључена двофакторска аутентификација (TOTP). Ако то нисте били ви — обратите се администратору.</p>`,
				Text:    "За ваш налог је укључена двофакторска аутентификација (TOTP). Ако то нисте били ви — обратите се администратору.",
			},
		},
	},
	"account.totp_disabled": {
		Key: "account.totp_disabled", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Two-factor authentication disabled",
				HTML:    `<h1>Two-factor authentication disabled</h1><p>Two-factor authentication (TOTP) has been turned off for your account. If this wasn't you, contact your administrator immediately.</p>`,
				Text:    "Two-factor authentication (TOTP) has been turned off for your account. If this wasn't you, contact your administrator immediately.",
			},
			"ru": {
				Subject: "Двухфакторная аутентификация отключена",
				HTML:    `<h1>Двухфакторная аутентификация отключена</h1><p>Для вашего аккаунта отключена двухфакторная аутентификация (TOTP). Если это были не вы — немедленно обратитесь к администратору.</p>`,
				Text:    "Для вашего аккаунта отключена двухфакторная аутентификация (TOTP). Если это были не вы — немедленно обратитесь к администратору.",
			},
			"de": {
				Subject: "Zwei-Faktor-Authentifizierung deaktiviert",
				HTML:    `<h1>Zwei-Faktor-Authentifizierung deaktiviert</h1><p>Die Zwei-Faktor-Authentifizierung (TOTP) wurde für Ihr Konto deaktiviert. Falls Sie das nicht waren, wenden Sie sich sofort an Ihren Administrator.</p>`,
				Text:    "Die Zwei-Faktor-Authentifizierung (TOTP) wurde für Ihr Konto deaktiviert. Falls Sie das nicht waren, wenden Sie sich sofort an Ihren Administrator.",
			},
			"uk": {
				Subject: "Двофакторну автентифікацію вимкнено",
				HTML:    `<h1>Двофакторну автентифікацію вимкнено</h1><p>Для вашого облікового запису вимкнено двофакторну автентифікацію (TOTP). Якщо це були не ви — негайно зверніться до адміністратора.</p>`,
				Text:    "Для вашого облікового запису вимкнено двофакторну автентифікацію (TOTP). Якщо це були не ви — негайно зверніться до адміністратора.",
			},
			"fr": {
				Subject: "Authentification à deux facteurs désactivée",
				HTML:    `<h1>Authentification à deux facteurs désactivée</h1><p>L'authentification à deux facteurs (TOTP) a été désactivée pour votre compte. Si ce n'était pas vous, contactez immédiatement votre administrateur.</p>`,
				Text:    "L'authentification à deux facteurs (TOTP) a été désactivée pour votre compte. Si ce n'était pas vous, contactez immédiatement votre administrateur.",
			},
			"es": {
				Subject: "Autenticación de dos factores desactivada",
				HTML:    `<h1>Autenticación de dos factores desactivada</h1><p>Se ha desactivado la autenticación de dos factores (TOTP) en tu cuenta. Si no fuiste tú, contacta de inmediato con tu administrador.</p>`,
				Text:    "Se ha desactivado la autenticación de dos factores (TOTP) en tu cuenta. Si no fuiste tú, contacta de inmediato con tu administrador.",
			},
			"sr": {
				Subject: "Двофакторска аутентификација искључена",
				HTML:    `<h1>Двофакторска аутентификација искључена</h1><p>За ваш налог је искључена двофакторска аутентификација (TOTP). Ако то нисте били ви — одмах се обратите администратору.</p>`,
				Text:    "За ваш налог је искључена двофакторска аутентификација (TOTP). Ако то нисте били ви — одмах се обратите администратору.",
			},
		},
	},
	"device.paired": {
		Key: "device.paired", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "A new app or device was connected",
				HTML:    `<h1>New device connected</h1><p>An app or device ("{{.Name}}") was connected to your account. If this wasn't you, remove it in your settings and contact your administrator.</p>`,
				Text:    "An app or device ({{.Name}}) was connected to your account. If this wasn't you, remove it in your settings and contact your administrator.",
			},
			"ru": {
				Subject: "Подключено новое приложение или устройство",
				HTML:    `<h1>Подключено новое устройство</h1><p>К вашему аккаунту подключено приложение или устройство («{{.Name}}»). Если это были не вы — удалите его в настройках и обратитесь к администратору.</p>`,
				Text:    "К вашему аккаунту подключено приложение или устройство ({{.Name}}). Если это были не вы — удалите его в настройках и обратитесь к администратору.",
			},
			"de": {
				Subject: "Eine neue App oder ein neues Gerät wurde verbunden",
				HTML:    `<h1>Neues Gerät verbunden</h1><p>Eine App oder ein Gerät («<b>{{.Name}}</b>») wurde mit Ihrem Konto verbunden. Falls Sie das nicht waren, entfernen Sie es in den Einstellungen und wenden Sie sich an Ihren Administrator.</p>`,
				Text:    "Eine App oder ein Gerät ({{.Name}}) wurde mit Ihrem Konto verbunden. Falls Sie das nicht waren, entfernen Sie es in den Einstellungen.",
			},
			"uk": {
				Subject: "Підключено новий застосунок або пристрій",
				HTML:    `<h1>Підключено новий пристрій</h1><p>До вашого облікового запису підключено застосунок або пристрій («<b>{{.Name}}</b>»). Якщо це були не ви — вилучіть його в налаштуваннях і зверніться до адміністратора.</p>`,
				Text:    "До вашого облікового запису підключено застосунок або пристрій ({{.Name}}). Якщо це були не ви — вилучіть його в налаштуваннях.",
			},
			"fr": {
				Subject: "Une nouvelle application ou un nouvel appareil a été connecté",
				HTML:    `<h1>Nouvel appareil connecté</h1><p>Une application ou un appareil («<b>{{.Name}}</b>») a été connecté à votre compte. Si ce n'était pas vous, supprimez-le dans les paramètres et contactez votre administrateur.</p>`,
				Text:    "Une application ou un appareil ({{.Name}}) a été connecté à votre compte. Si ce n'était pas vous, supprimez-le dans les paramètres.",
			},
			"es": {
				Subject: "Se conectó una nueva aplicación o dispositivo",
				HTML:    `<h1>Nuevo dispositivo conectado</h1><p>Una aplicación o dispositivo («<b>{{.Name}}</b>») se ha conectado a tu cuenta. Si no fuiste tú, elimínalo en los ajustes y contacta con tu administrador.</p>`,
				Text:    "Una aplicación o dispositivo ({{.Name}}) se ha conectado a tu cuenta. Si no fuiste tú, elimínalo en los ajustes.",
			},
			"sr": {
				Subject: "Повезана је нова апликација или уређај",
				HTML:    `<h1>Повезан нови уређај</h1><p>Апликација или уређај («<b>{{.Name}}</b>») је повезан са вашим налогом. Ако то нисте били ви — уклоните га у подешавањима и обратите се администратору.</p>`,
				Text:    "Апликација или уређај ({{.Name}}) је повезан са вашим налогом. Ако то нисте били ви — уклоните га у подешавањима.",
			},
		},
	},
	"account.profile_changed": {
		Key: "account.profile_changed", Category: Activity, Mandatory: false, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Your profile was updated",
				HTML:    `<h1>Profile updated</h1><p>Your profile information has been changed.</p>`,
				Text:    "Your profile information has been changed.",
			},
			"ru": {
				Subject: "Профиль обновлён",
				HTML:    `<h1>Профиль обновлён</h1><p>Данные вашего профиля были изменены.</p>`,
				Text:    "Данные вашего профиля были изменены.",
			},
			"de": {
				Subject: "Ihr Profil wurde aktualisiert",
				HTML:    `<h1>Profil aktualisiert</h1><p>Ihre Profilinformationen wurden geändert.</p>`,
				Text:    "Ihre Profilinformationen wurden geändert.",
			},
			"uk": {
				Subject: "Ваш профіль оновлено",
				HTML:    `<h1>Профіль оновлено</h1><p>Дані вашого профілю було змінено.</p>`,
				Text:    "Дані вашого профілю було змінено.",
			},
			"fr": {
				Subject: "Votre profil a été mis à jour",
				HTML:    `<h1>Profil mis à jour</h1><p>Les informations de votre profil ont été modifiées.</p>`,
				Text:    "Les informations de votre profil ont été modifiées.",
			},
			"es": {
				Subject: "Tu perfil ha sido actualizado",
				HTML:    `<h1>Perfil actualizado</h1><p>La información de tu perfil ha sido modificada.</p>`,
				Text:    "La información de tu perfil ha sido modificada.",
			},
			"sr": {
				Subject: "Ваш профил је ажуриран",
				HTML:    `<h1>Профил ажуриран</h1><p>Подаци вашег профила су измењени.</p>`,
				Text:    "Подаци вашег профила су измењени.",
			},
		},
	},
	"account.passkey_added": {
		Key: "account.passkey_added", Category: Security, Mandatory: true, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "New passkey added to your account",
				HTML:    `<h1>New passkey</h1><p>A passkey has been added to your account. If this wasn't you, contact your administrator.</p>`,
				Text:    "A passkey has been added to your account. If this wasn't you, contact your administrator.",
			},
			"ru": {
				Subject: "Добавлен ключ входа",
				HTML:    `<h1>Новый ключ входа</h1><p>К вашему аккаунту добавлен ключ входа (passkey). Если это были не вы — обратитесь к администратору.</p>`,
				Text:    "К вашему аккаунту добавлен ключ входа. Если это были не вы — обратитесь к администратору.",
			},
			"de": {
				Subject: "Neuer Passkey zu Ihrem Konto hinzugefügt",
				HTML:    `<h1>Neuer Passkey</h1><p>Zu Ihrem Konto wurde ein Passkey hinzugefügt. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.</p>`,
				Text:    "Zu Ihrem Konto wurde ein Passkey hinzugefügt. Falls Sie das nicht waren, wenden Sie sich an Ihren Administrator.",
			},
			"uk": {
				Subject: "Додано ключ входу",
				HTML:    `<h1>Новий ключ входу</h1><p>До вашого облікового запису додано ключ входу (passkey). Якщо це були не ви — зверніться до адміністратора.</p>`,
				Text:    "До вашого облікового запису додано ключ входу. Якщо це були не ви — зверніться до адміністратора.",
			},
			"fr": {
				Subject: "Nouvelle passkey ajoutée à votre compte",
				HTML:    `<h1>Nouvelle passkey</h1><p>Une passkey a été ajoutée à votre compte. Si ce n'était pas vous, contactez votre administrateur.</p>`,
				Text:    "Une passkey a été ajoutée à votre compte. Si ce n'était pas vous, contactez votre administrateur.",
			},
			"es": {
				Subject: "Nueva passkey añadida a tu cuenta",
				HTML:    `<h1>Nueva passkey</h1><p>Se ha añadido una passkey a tu cuenta. Si no fuiste tú, contacta con tu administrador.</p>`,
				Text:    "Se ha añadido una passkey a tu cuenta. Si no fuiste tú, contacta con tu administrador.",
			},
			"sr": {
				Subject: "Додат нови кључ за пријаву",
				HTML:    `<h1>Нови кључ за пријаву</h1><p>Вашем налогу је додат кључ за пријаву (passkey). Ако то нисте били ви — обратите се администратору.</p>`,
				Text:    "Вашем налогу је додат кључ за пријаву. Ако то нисте били ви — обратите се администратору.",
			},
		},
	},
	"sync.failed": {
		Key: "sync.failed", Category: Activity, Mandatory: false, DefaultChannels: []string{"email"},
		Templates: map[string]Template{
			"en": {
				Subject: "Sync failed",
				HTML:    `<h1>Sync failed</h1><p>An error occurred during synchronization: {{.Detail}}.</p>`,
				Text:    "An error occurred during synchronization: {{.Detail}}.",
			},
			"ru": {
				Subject: "Сбой синхронизации",
				HTML:    `<h1>Сбой синхронизации</h1><p>При синхронизации возникла ошибка: {{.Detail}}.</p>`,
				Text:    "При синхронизации возникла ошибка: {{.Detail}}.",
			},
			"de": {
				Subject: "Synchronisierung fehlgeschlagen",
				HTML:    `<h1>Synchronisierung fehlgeschlagen</h1><p>Bei der Synchronisierung ist ein Fehler aufgetreten: {{.Detail}}.</p>`,
				Text:    "Bei der Synchronisierung ist ein Fehler aufgetreten: {{.Detail}}.",
			},
			"uk": {
				Subject: "Збій синхронізації",
				HTML:    `<h1>Збій синхронізації</h1><p>Під час синхронізації сталася помилка: {{.Detail}}.</p>`,
				Text:    "Під час синхронізації сталася помилка: {{.Detail}}.",
			},
			"fr": {
				Subject: "Échec de la synchronisation",
				HTML:    `<h1>Échec de la synchronisation</h1><p>Une erreur s'est produite lors de la synchronisation : {{.Detail}}.</p>`,
				Text:    "Une erreur s'est produite lors de la synchronisation : {{.Detail}}.",
			},
			"es": {
				Subject: "Error de sincronización",
				HTML:    `<h1>Error de sincronización</h1><p>Se produjo un error durante la sincronización: {{.Detail}}.</p>`,
				Text:    "Se produjo un error durante la sincronización: {{.Detail}}.",
			},
			"sr": {
				Subject: "Неуспела синхронизација",
				HTML:    `<h1>Неуспела синхронизација</h1><p>Дошло је до грешке током синхронизације: {{.Detail}}.</p>`,
				Text:    "Дошло је до грешке током синхронизације: {{.Detail}}.",
			},
		},
	},
}
