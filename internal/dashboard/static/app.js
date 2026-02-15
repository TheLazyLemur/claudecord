// Dashboard WebSocket client

const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const WS_URL = `${WS_PROTOCOL}//${window.location.host}/ws`;

let ws = null;
let reconnectTimer = null;
let pendingPermission = null;
let currentSkill = null;
let skillFiles = [];

// DOM elements
const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const sendBtn = document.getElementById('sendBtn');
const newSessionBtn = document.getElementById('newSessionBtn');
const sessionInfo = document.getElementById('sessionInfo');
const sessionID = document.getElementById('sessionID');
const typingIndicator = document.getElementById('typingIndicator');
const logsContainer = document.getElementById('logsContainer');
const clearLogsBtn = document.getElementById('clearLogsBtn');
const skillsList = document.getElementById('skillsList');
const refreshSkillsBtn = document.getElementById('refreshSkillsBtn');
const newSkillBtn = document.getElementById('newSkillBtn');
const skillSearch = document.getElementById('skillSearch');

let allSkills = [];

// Permission modal
const permissionModal = document.getElementById('permissionModal');
const permissionPrompt = document.getElementById('permissionPrompt');
const permApproveBtn = document.getElementById('permApproveBtn');
const permDenyBtn = document.getElementById('permDenyBtn');

// WhatsApp QR
const whatsappQR = document.getElementById('whatsappQR');
const qrCanvas = document.getElementById('qrCanvas');
const qrStatus = document.getElementById('qrStatus');

// Skill modal
const skillModal = document.getElementById('skillModal');
const skillModalTitle = document.getElementById('skillModalTitle');
const closeSkillModalBtn = document.getElementById('closeSkillModalBtn');
const skillContent = document.getElementById('skillContent');
const skillContentTab = document.getElementById('skillContentTab');
const skillFilesTab = document.getElementById('skillFilesTab');
const skillFilesList = document.getElementById('skillFilesList');
const fileUploadInput = document.getElementById('fileUploadInput');
const saveSkillBtn = document.getElementById('saveSkillBtn');
const cancelSkillBtn = document.getElementById('cancelSkillBtn');
const skillTabs = document.querySelectorAll('.skill-tab');

// Connect WebSocket
function connect() {
  ws = new WebSocket(WS_URL);

  ws.onopen = () => {
    console.log('WS connected');
    addLog('INFO', 'Connected to dashboard');
    // Request skills list
    send({ type: 'get_skills' });
  };

  ws.onclose = () => {
    console.log('WS closed');
    scheduleReconnect();
  };

  ws.onerror = (err) => {
    console.error('WS error', err);
  };

  ws.onmessage = (event) => {
    // Handle multiple messages (newline separated)
    const messages = event.data.split('\n').filter(Boolean);
    for (const data of messages) {
      try {
        const msg = JSON.parse(data);
        handleMessage(msg);
      } catch (e) {
        console.error('Parse error', e);
      }
    }
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect();
  }, 2000);
}

function send(msg) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg));
  }
}

// Message handlers
function handleMessage(msg) {
  switch (msg.type) {
    case 'log':
      addLog(msg.level, msg.msg, msg.time);
      break;

    case 'chat':
      addChatMessage(msg.role, msg.content);
      break;

    case 'typing':
      setTyping(msg.active);
      break;

    case 'session':
      updateSession(msg.active, msg.sessionID);
      break;

    case 'permission':
      showPermissionModal(msg.id, msg.prompt);
      break;

    case 'skills':
      renderSkillsList(msg.skills || []);
      break;

    case 'skill_detail':
      showSkillEditor(msg.name, msg.content, msg.files || []);
      break;

    case 'whatsapp_qr':
      handleWhatsAppQR(msg.content);
      break;
  }
}

// Chat
function addChatMessage(role, content) {
  const div = document.createElement('div');
  div.className = role === 'user'
    ? 'ml-auto max-w-[80%] bg-zinc-800 rounded-lg px-4 py-2'
    : 'mr-auto max-w-[80%] bg-zinc-900 border border-zinc-800 rounded-lg px-4 py-2';

  const pre = document.createElement('pre');
  pre.className = 'whitespace-pre-wrap text-sm';
  pre.textContent = content;
  div.appendChild(pre);

  chatMessages.appendChild(div);
  chatMessages.scrollTop = chatMessages.scrollHeight;
}

function sendChat() {
  const text = chatInput.value.trim();
  if (!text) return;

  send({ type: 'chat', content: text });
  chatInput.value = '';
}

function setTyping(active) {
  typingIndicator.classList.toggle('hidden', !active);
  chatMessages.scrollTop = chatMessages.scrollHeight;
}

// Session
function updateSession(active, id) {
  if (active) {
    sessionInfo.classList.remove('hidden');
    sessionID.textContent = id ? id.slice(0, 8) : '-';
  } else {
    sessionInfo.classList.add('hidden');
  }
}

function newSession() {
  send({ type: 'new_session', workDir: '' });
  chatMessages.innerHTML = '';
}

// Logs
function addLog(level, msg, time) {
  const div = document.createElement('div');
  div.className = 'flex gap-3';

  const levelClass = {
    'DEBUG': 'text-zinc-600',
    'INFO': 'text-zinc-400',
    'WARN': 'text-amber-500',
    'ERROR': 'text-red-500',
  }[level] || 'text-zinc-400';

  div.innerHTML = `
    <span class="text-zinc-600 shrink-0">${time ? new Date(time).toLocaleTimeString() : new Date().toLocaleTimeString()}</span>
    <span class="${levelClass} shrink-0 w-12">${level}</span>
    <span class="text-zinc-300 break-all">${escapeHtml(msg)}</span>
  `;

  logsContainer.appendChild(div);
  logsContainer.scrollTop = logsContainer.scrollHeight;

  // Limit log entries
  while (logsContainer.children.length > 500) {
    logsContainer.removeChild(logsContainer.firstChild);
  }
}

function clearLogs() {
  logsContainer.innerHTML = '';
}

// Permission modal
function showPermissionModal(id, prompt) {
  pendingPermission = id;
  permissionPrompt.textContent = prompt;
  permissionModal.classList.remove('hidden');
}

function hidePermissionModal() {
  permissionModal.classList.add('hidden');
  pendingPermission = null;
}

function respondPermission(approved) {
  if (!pendingPermission) return;
  send({ type: 'permission_response', id: pendingPermission, approved });
  hidePermissionModal();
}

// Skills
function renderSkillsList(skills, filter = '') {
  allSkills = skills;
  filterSkills(filter);
}

function filterSkills(filter = '') {
  const lowerFilter = filter.toLowerCase();
  const filtered = allSkills.filter(sk =>
    sk.name.toLowerCase().includes(lowerFilter) ||
    sk.description.toLowerCase().includes(lowerFilter)
  );

  skillsList.innerHTML = '';
  for (const sk of filtered) {
    const div = document.createElement('div');
    div.className = 'px-3 py-2 rounded hover:bg-zinc-800 cursor-pointer transition-colors';
    div.innerHTML = `
      <div class="text-sm text-zinc-100">${escapeHtml(sk.name)}</div>
      <div class="text-xs text-zinc-500 truncate">${escapeHtml(sk.description)}</div>
    `;
    div.onclick = () => openSkill(sk.name);
    skillsList.appendChild(div);
  }
}

function openSkill(name) {
  send({ type: 'get_skill', name });
}

function newSkill() {
  const name = prompt('Skill name (lowercase, hyphens only):');
  if (!name) return;

  // Validate name format
  if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(name)) {
    alert('Invalid name. Use lowercase letters, numbers, and hyphens only.');
    return;
  }

  // Show editor with template
  const template = `---
name: ${name}
description: Description here
---

# ${name}

Instructions for the skill...
`;

  showSkillEditor(name, template, []);
}

function showSkillEditor(name, content, files) {
  currentSkill = name;
  skillFiles = [];

  skillModalTitle.textContent = `Edit: ${name}`;
  skillContent.value = content;

  renderSkillFiles(files);
  switchSkillTab('content');

  skillModal.classList.remove('hidden');
}

function renderSkillFiles(files) {
  skillFilesList.innerHTML = '';
  for (const f of files) {
    const div = document.createElement('div');
    div.className = 'flex items-center justify-between bg-zinc-800 rounded px-3 py-2';
    div.innerHTML = `
      <div>
        <div class="text-sm text-zinc-100">${escapeHtml(f.path)}</div>
        <div class="text-xs text-zinc-500">${formatBytes(f.size)}</div>
      </div>
      <button class="delete-file-btn text-zinc-500 hover:text-red-400 transition-colors" data-path="${escapeHtml(f.path)}">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
        </svg>
      </button>
    `;
    div.querySelector('.delete-file-btn').onclick = () => deleteSkillFile(f.path);
    skillFilesList.appendChild(div);
  }
}

function switchSkillTab(tab) {
  skillTabs.forEach(t => {
    const active = t.dataset.tab === tab;
    t.classList.toggle('border-zinc-100', active);
    t.classList.toggle('text-zinc-100', active);
  });
  skillContentTab.classList.toggle('hidden', tab !== 'content');
  skillFilesTab.classList.toggle('hidden', tab !== 'files');
}

function hideSkillModal() {
  skillModal.classList.add('hidden');
  currentSkill = null;
  skillFiles = [];
}

function saveSkill() {
  if (!currentSkill) return;
  send({
    type: 'save_skill',
    name: currentSkill,
    content: skillContent.value,
    files: skillFiles
  });
  hideSkillModal();
}

function deleteSkillFile(path) {
  if (!currentSkill) return;
  if (!confirm(`Delete ${path}?`)) return;
  send({ type: 'delete_skill_file', name: currentSkill, path });
}

function handleFileUpload(files) {
  for (const file of files) {
    const reader = new FileReader();
    reader.onload = (e) => {
      // Determine subdir based on extension
      let subdir = 'assets';
      if (file.name.endsWith('.sh') || file.name.endsWith('.py') || file.name.endsWith('.js')) {
        subdir = 'scripts';
      } else if (file.name.endsWith('.md') || file.name.endsWith('.txt') || file.name.endsWith('.json')) {
        subdir = 'references';
      }

      skillFiles.push({
        path: `${subdir}/${file.name}`,
        content: e.target.result
      });

      // Add to UI
      const div = document.createElement('div');
      div.className = 'flex items-center justify-between bg-zinc-700 rounded px-3 py-2';
      div.innerHTML = `
        <div>
          <div class="text-sm text-zinc-100">${subdir}/${escapeHtml(file.name)}</div>
          <div class="text-xs text-emerald-400">New</div>
        </div>
      `;
      skillFilesList.appendChild(div);
    };
    reader.readAsText(file);
  }
}

// WhatsApp QR
function handleWhatsAppQR(content) {
  if (content === 'success') {
    qrStatus.textContent = 'Paired';
    qrCanvas.classList.add('hidden');
    setTimeout(() => whatsappQR.classList.add('hidden'), 2000);
    return;
  }
  if (content === 'timeout') {
    qrStatus.textContent = 'QR expired';
    qrCanvas.classList.add('hidden');
    return;
  }
  if (content.startsWith('err')) {
    qrStatus.textContent = content;
    qrCanvas.classList.add('hidden');
    return;
  }
  // Render QR code
  whatsappQR.classList.remove('hidden');
  qrCanvas.classList.remove('hidden');
  qrStatus.textContent = '';
  const ctx = qrCanvas.getContext('2d');
  ctx.clearRect(0, 0, qrCanvas.width, qrCanvas.height);
  QRCode.toCanvas(qrCanvas, content, { width: 256, margin: 1 })
    .catch((e) => { qrStatus.textContent = 'QR render error: ' + e.message; });
}

// Utility
function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function formatBytes(bytes) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

// Event listeners
sendBtn.onclick = sendChat;
chatInput.onkeydown = (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendChat();
  }
};

newSessionBtn.onclick = newSession;
clearLogsBtn.onclick = clearLogs;
refreshSkillsBtn.onclick = () => send({ type: 'get_skills' });
newSkillBtn.onclick = newSkill;
skillSearch.oninput = () => filterSkills(skillSearch.value);

permApproveBtn.onclick = () => respondPermission(true);
permDenyBtn.onclick = () => respondPermission(false);

closeSkillModalBtn.onclick = hideSkillModal;
cancelSkillBtn.onclick = hideSkillModal;
saveSkillBtn.onclick = saveSkill;

skillTabs.forEach(t => {
  t.onclick = () => switchSkillTab(t.dataset.tab);
});

fileUploadInput.onchange = (e) => handleFileUpload(e.target.files);

// Drag and drop for file upload
skillFilesTab.ondragover = (e) => {
  e.preventDefault();
  e.currentTarget.classList.add('bg-zinc-800');
};
skillFilesTab.ondragleave = (e) => {
  e.currentTarget.classList.remove('bg-zinc-800');
};
skillFilesTab.ondrop = (e) => {
  e.preventDefault();
  e.currentTarget.classList.remove('bg-zinc-800');
  handleFileUpload(e.dataTransfer.files);
};

// Close modals on backdrop click
permissionModal.onclick = (e) => {
  if (e.target === permissionModal) hidePermissionModal();
};
skillModal.onclick = (e) => {
  if (e.target === skillModal) hideSkillModal();
};

// Start
connect();
