# Over Melon

Melon is je Zermelo-rooster, maar dan overzichtelijk. Wijzigingen en uitval zie je in één oogopslag, je kunt meerdere accounts naast elkaar gebruiken, en je rooster werkt ook in je agenda, via een API en in AI-assistenten.

Melon is geen onderdeel van Zermelo of van je school. Het is een los programma dat met jouw toestemming je rooster bij Zermelo ophaalt. De code is open: [github.com/MatthiasLubbertsen/melon](https://github.com/MatthiasLubbertsen/melon).

## Hoe het werkt

1. In Zermelo maak je een **koppelcode** aan (in de Zermelo-app of op het portaal van je school, onder *Koppelingen*).
2. Melon ruilt die code één keer in voor een **token**: een sleutel waarmee Melon je rooster mag lezen. Dat inruilen is het enige wat Melon ooit naar Zermelo stuurt; daarna leest Melon alleen.
3. Als je je rooster opent, vraagt Melon het bij Zermelo op. Een antwoord wordt 30 seconden bewaard, zodat Zermelo niet overspoeld wordt. Ligt Zermelo eruit, dan zie je het laatste rooster dat Melon had (hooguit 30 minuten oud), duidelijk gemarkeerd.
4. Wat Zermelo stuurt, maakt Melon leesbaar: vakken krijgen hun volledige naam, docentcodes een naam als je die invult, en wijzigingen worden kort uitgelegd.

## Je profiel

Je profiel begint als de browser waarin je Melon opent. Daarin kun je zoveel Zermelo-accounts koppelen als je wilt. Stel je een **pincode** in, dan kun je op een ander apparaat inloggen met je Zermelo-gebruikersnummer en die pincode.

Een pincode is kort, dus Melon beschermt tegen raden: na vijf foute pogingen kan er een kwartier niet worden ingelogd, en elke volgende fout verdubbelt die tijd, tot een dag.

## Privacy

- **Je Zermelo-token** wordt versleuteld opgeslagen (AES-256-GCM) en gaat nooit naar je browser, de API of apps. Alleen een beheerder kan het inzien als die je helpt (zie hieronder).
- **Pincodes, sessies, API-sleutels en app-koppelingen** worden alleen als hash bewaard. Een API-sleutel zie je één keer; Melon kan hem daarna zelf ook niet meer laten zien.
- **Agenda-links** bevatten een geheime code. Je kunt een nieuwe aanmaken, dan werkt de oude niet meer.
- **Geen tracking.** Melon gebruikt geen advertenties of analysediensten en zet alleen een cookie om je ingelogd te houden. Je taal en je laatst gekozen account onthoudt je eigen browser.
- **Gebruik wordt alleen opgeteld.** Melon telt per dag hoeveel roosters er via de app, de API, apps en agenda's zijn opgevraagd, niet wie dat deed. Je IP-adres wordt alleen kort in het geheugen gebruikt om misbruik af te remmen, en niet opgeslagen.

### Wat beheerders kunnen zien

De beheerders van deze Melon-server kunnen, om mensen te helpen, profielen bekijken: welke accounts eraan gekoppeld zijn, of er een pincode is, en welke API-sleutels en apps er zijn (zonder de sleutels zelf). Bij een probleem kunnen ze Melon tijdelijk bekijken zoals jij het ziet, of je Zermelo-token bekijken en controleren. Alles wat een beheerder zo doet, komt in een logboek.

## Alles wissen

Onder **Instellingen → Accounts → Alles wissen** verwijder je je profiel met alles erin: gekoppelde accounts en hun tokens, pincode, API-sleutels, verbonden apps, agenda-links en de namen die je gaf. Een los account ontkoppelen kan ook; dan vergeet Melon alleen dat account en zijn token.

Wil je ook dat Zermelo de koppeling vergeet, verwijder hem dan in Zermelo onder *Koppelingen*.
