// voice.js — Speech recognition and TTS

var voiceMode = false;
var recognition = null;
var accumulated = '';
var silenceTimer = null;
var currentAudio = null;
var voiceSettings = { lang: 'zh-CN', voiceURI: '', rate: 1, pitch: 1 };

try {
  var saved = JSON.parse(localStorage.getItem('iamhuman-voice') || '{}');
  if (saved.lang) voiceSettings.lang = saved.lang;
  if (saved.voiceURI) voiceSettings.voiceURI = saved.voiceURI;
  if (saved.rate) voiceSettings.rate = saved.rate;
  if (saved.pitch) voiceSettings.pitch = saved.pitch;
} catch(e) {}

function saveVoiceSettings() {
  try { localStorage.setItem('iamhuman-voice', JSON.stringify(voiceSettings)); } catch(e) {}
}

var SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
var micBtn = document.getElementById('mic-btn');

micBtn.addEventListener('click', function() {
  if (!voiceMode) { startListening(); return; }
  stopListening();
  voiceMode = false;
  updateMicButton();
  state.name = 'idle';
  msgInput.placeholder = t('input.placeholder');
});

function updateMicButton() {
  micBtn.classList.remove('mic-active', 'mic-speaking');
  if (state.name === 'speaking') micBtn.classList.add('mic-speaking');
  else if (voiceMode) micBtn.classList.add('mic-active');
}

function startListening() {
  if (!SpeechRecognition) { console.error('Speech recognition not supported'); return; }
  voiceMode = true;
  updateMicButton();
  state.name = 'listening';
  msgInput.placeholder = t('status.listening');

  if (state.name === 'speaking') { speechSynthesis.cancel(); if (currentAudio) { currentAudio.pause(); currentAudio = null; } }
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  accumulated = '';

  recognition = new SpeechRecognition();
  recognition.continuous = true;
  recognition.interimResults = true;
  recognition.lang = voiceSettings.lang;

  recognition.onresult = function(e) {
    var interim = '', finalText = '';
    for (var i = e.resultIndex; i < e.results.length; i++) {
      var r = e.results[i];
      if (r.isFinal) finalText += r[0].transcript;
      else interim += r[0].transcript;
    }
    if (finalText) accumulated += finalText;
    msgInput.value = accumulated + (interim ? ' ' + interim : '');
    resetSilenceTimer();
  };

  recognition.onspeechend = function() {
    resetSilenceTimer();
  };

  recognition.onerror = function(e) {
    if (e.error === 'no-speech') {
      if (voiceMode && state.name !== 'speaking') {
        try { recognition.start(); } catch(_) {}
      }
      return;
    }
    if (e.error === 'aborted') return;
    // Only turn off mic for permission / unavailable errors
    if (e.error === 'not-allowed' || e.error === 'service-not-allowed') {
      console.error('Speech recognition unavailable: ' + e.error);
      stopListening();
      voiceMode = false;
      updateMicButton();
      state.name = 'idle';
      msgInput.placeholder = t('input.placeholder');
      return;
    }
    // Network / audio-capture errors: retry once
    console.warn('Speech recognition error: ' + e.error + ', retrying...');
    if (voiceMode) {
      setTimeout(function() {
        if (!voiceMode) return;
        try { recognition.start(); } catch(_) {}
      }, 500);
    }
  };

  function tryStart(retry) {
    try {
      recognition.start();
    } catch(e) {
      if (retry) {
        setTimeout(function() {
          if (!voiceMode) return;
          try { recognition.start(); } catch(e2) {}
        }, 500);
      } else {
        setTimeout(function() { return tryStart(true); }, 300);
      }
    }
  }

  tryStart(false);
}

function resetSilenceTimer() {
  if (silenceTimer) clearTimeout(silenceTimer);
  silenceTimer = setTimeout(function() {
    if (accumulated.trim()) stopVoiceAndSend();
  }, 2000);
}

function stopVoiceAndSend() {
  if (silenceTimer) clearTimeout(silenceTimer);
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  updateMicButton();
  state.name = 'idle';
  msgInput.placeholder = t('status.ai_thinking');
  if (accumulated.trim()) msgInput.value = accumulated.trim();
  if (msgInput.value) sendMsg();
  accumulated = '';
}

function stopListening() {
  if (silenceTimer) clearTimeout(silenceTimer);
  if (recognition) { try { recognition.abort(); } catch(e) {} }
  accumulated = '';
  msgInput.value = '';
}

function stripHtml(html) {
  var tmp = document.createElement('div');
  tmp.innerHTML = html;
  return tmp.textContent || tmp.innerText || '';
}

// ── TTS speak (called from SSE speaking event) ──

function speak(text) {
  var clean = stripHtml(text).trim();
  if (!clean) return;
  speechSynthesis.cancel();
  var u = new SpeechSynthesisUtterance(clean);
  u.rate = voiceSettings.rate;
  u.pitch = voiceSettings.pitch;
  u.lang = voiceSettings.lang || 'zh-CN';
  if (voiceSettings.voiceURI) {
    var voices = speechSynthesis.getVoices();
    for (var i = 0; i < voices.length; i++) {
      if (voices[i].voiceURI === voiceSettings.voiceURI) { u.voice = voices[i]; break; }
    }
  }
  u.onstart = function() {
    state.name = 'speaking'; updateMicButton();
    showTTSBar();
  };
  u.onboundary = function(e) {
    if (clean.length > 0) {
      updateTTSBar(Math.round((e.charIndex / clean.length) * 100));
    }
  };
  u.onend = function() {
    state.name = 'idle'; updateMicButton();
    hideTTSBar();
  };
  u.onerror = function(e) {
    if (e.error === 'canceled' || e.error === 'interrupted') { hideTTSBar(); return; }
    console.error('TTS error: ' + e.error);
    state.name = 'idle'; updateMicButton();
    hideTTSBar();
  };
  speechSynthesis.speak(u);
}

// ── Speak toggle ──

function toggleSpeak() {
  speakEnabled = !speakEnabled;
  updateSpeakButton();
  if (!speakEnabled && state.name === 'speaking') {
    speechSynthesis.cancel();
    state.name = 'idle';
    updateMicButton();
  }
  fetch(SERVER_URL + '/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ speak_enabled: speakEnabled })
  }).catch(function(e) { console.error('speak toggle error:', e); });
}

function updateSpeakButton() {
  var btn = document.getElementById('speak-btn');
  if (btn) btn.classList.toggle('active', speakEnabled);
}

// Init speak button state
(function() {
  fetch(SERVER_URL + '/api/settings')
    .then(function(r) { return r.json(); })
    .then(function(cfg) {
      speakEnabled = cfg.speak_enabled !== false;
      updateSpeakButton();
    })
    .catch(function() { updateSpeakButton(); });
})();

// ── Voice settings UI ──

function populateVoiceList() {
  var voices = speechSynthesis.getVoices();
  var sel = document.getElementById('set-tts-voice');
  if (!sel) return;
  var html = '';
  for (var i = 0; i < voices.length; i++) {
    var v = voices[i];
    var selected = v.voiceURI === voiceSettings.voiceURI ? ' selected' : '';
    html += '<option value="' + v.voiceURI + '"' + selected + '>' + v.name + ' (' + v.lang + ')</option>';
  }
  if (!html) html = '<option value="">(none)</option>';
  sel.innerHTML = html;
}
speechSynthesis.onvoiceschanged = populateVoiceList;
setTimeout(populateVoiceList, 500);

function applyVoiceSettingsFromUI() {
  var langEl = document.getElementById('set-stt-lang');
  var voiceEl = document.getElementById('set-tts-voice');
  var rateEl = document.getElementById('set-tts-rate');
  var pitchEl = document.getElementById('set-tts-pitch');
  if (langEl) voiceSettings.lang = langEl.value;
  if (voiceEl) voiceSettings.voiceURI = voiceEl.value;
  if (rateEl) voiceSettings.rate = parseFloat(rateEl.value);
  if (pitchEl) voiceSettings.pitch = parseFloat(pitchEl.value);
  saveVoiceSettings();
}

function loadVoiceSettingsToUI() {
  populateVoiceList();
  var langEl = document.getElementById('set-stt-lang');
  var rateEl = document.getElementById('set-tts-rate');
  var pitchEl = document.getElementById('set-tts-pitch');
  if (langEl) langEl.value = voiceSettings.lang;
  if (rateEl) { rateEl.value = voiceSettings.rate; }
  if (pitchEl) { pitchEl.value = voiceSettings.pitch; }
}

// Attach voice settings listeners
var rateSlider = document.getElementById('set-tts-rate');
var pitchSlider = document.getElementById('set-tts-pitch');
if (rateSlider) {
  rateSlider.addEventListener('input', function() {
    voiceSettings.rate = parseFloat(this.value);
    saveVoiceSettings();
  });
}
if (pitchSlider) {
  pitchSlider.addEventListener('input', function() {
    voiceSettings.pitch = parseFloat(this.value);
    saveVoiceSettings();
  });
}
var sttLang = document.getElementById('set-stt-lang');
if (sttLang) { sttLang.addEventListener('change', function() { voiceSettings.lang = this.value; saveVoiceSettings(); }); }
var ttsVoice = document.getElementById('set-tts-voice');
if (ttsVoice) { ttsVoice.addEventListener('change', function() { voiceSettings.voiceURI = this.value; saveVoiceSettings(); }); }

// Hook into settings panel open to refresh voice list
var origToggleSettings = toggleSettings;
toggleSettings = function() {
  origToggleSettings();
  if (document.getElementById('settings-panel').classList.contains('open')) {
    setTimeout(loadVoiceSettingsToUI, 200);
  }
};
