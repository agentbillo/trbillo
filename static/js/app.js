// Trbillo Kanban Client Application
document.addEventListener('DOMContentLoaded', () => {
  // --- STATE ---
  const state = {
    user: null,
    boards: [],
    activeBoard: null,
    activeCard: null,
    dnd: null
  };

  // --- DOM ELEMENTS ---
  const el = {
    authContainer: document.getElementById('auth-container'),
    dashboardContainer: document.getElementById('dashboard-container'),
    loginForm: document.getElementById('login-form'),
    registerForm: document.getElementById('register-form'),
    showSignup: document.getElementById('show-signup'),
    showLogin: document.getElementById('show-login'),
    loginUsername: document.getElementById('login-username'),
    loginPassword: document.getElementById('login-password'),
    registerUsername: document.getElementById('register-username'),
    registerEmail: document.getElementById('register-email'),
    registerPassword: document.getElementById('register-password'),

    sidebar: document.getElementById('sidebar'),
    sidebarToggle: document.getElementById('sidebar-toggle'),
    boardsList: document.getElementById('boards-list'),
    createBoardBtn: document.getElementById('create-board-btn'),
    welcomeCreateBoardBtn: document.getElementById('welcome-create-board-btn'),
    userAvatar: document.getElementById('user-avatar'),
    userName: document.getElementById('user-name'),
    logoutBtn: document.getElementById('logout-btn'),

    boardHeader: document.getElementById('board-header'),
    boardTitle: document.getElementById('board-title'),
    boardDesc: document.getElementById('board-desc'),
    boardMembersList: document.getElementById('board-members-list'),
    inviteMemberBtn: document.getElementById('invite-member-btn'),
    activityLogToggle: document.getElementById('activity-log-toggle'),
    boardSettingsBtn: document.getElementById('board-settings-btn'),
    deleteBoardBtn: document.getElementById('delete-board-btn'),
    leaveBoardBtn: document.getElementById('leave-board-btn'),
    boardOwner: document.getElementById('board-owner'),
    welcomeMessage: document.getElementById('welcome-message'),
    listsContainer: document.getElementById('lists-container'),
    kanbanCanvas: document.getElementById('kanban-canvas'),

    activityPanel: document.getElementById('activity-panel'),
    activityPanelClose: document.getElementById('activity-panel-close'),
    activityList: document.getElementById('activity-list'),

    // Modals
    createBoardModal: document.getElementById('create-board-modal'),
    createBoardForm: document.getElementById('create-board-form'),
    newBoardName: document.getElementById('new-board-name'),
    newBoardDesc: document.getElementById('new-board-desc'),

    inviteMemberModal: document.getElementById('invite-member-modal'),
    inviteMemberForm: document.getElementById('invite-member-form'),
    inviteUsernameInput: document.getElementById('invite-username-input'),
    collaboratorsSection: document.getElementById('collaborators-section'),
    collaboratorsList: document.getElementById('collaborators-list'),

    boardSettingsModal: document.getElementById('board-settings-modal'),
    boardSettingsForm: document.getElementById('board-settings-form'),
    settingsBoardName: document.getElementById('settings-board-name'),
    settingsBoardDesc: document.getElementById('settings-board-desc'),
    settingsMembersList: document.getElementById('settings-members-list'),
    themePicker: document.querySelectorAll('.theme-option'),
    myBoardsFilter: document.getElementById('my-boards-filter'),
    boardsSort: document.getElementById('boards-sort'),
    settingsBoardIcon: document.getElementById('settings-board-icon'),
    iconPresets: document.querySelectorAll('.icon-preset'),

    cardDetailModal: document.getElementById('card-detail-modal'),
    modalCardTitle: document.getElementById('modal-card-title'),
    modalListName: document.getElementById('modal-list-name'),
    modalCardDesc: document.getElementById('modal-card-desc'),
    saveDescBtn: document.getElementById('save-desc-btn'),
    checklistProgress: document.getElementById('checklist-progress'),
    checklistProgressBar: document.getElementById('checklist-progress-bar'),
    checklistItemsList: document.getElementById('checklist-items-list'),
    newChecklistTitle: document.getElementById('new-checklist-title'),
    addChecklistBtn: document.getElementById('add-checklist-btn'),
    newCommentContent: document.getElementById('new-comment-content'),
    postCommentBtn: document.getElementById('post-comment-btn'),
    commentsList: document.getElementById('comments-list'),
    modalCardDuedate: document.getElementById('modal-card-duedate'),
    saveDuedateBtn: document.getElementById('save-duedate-btn'),
    modalCardLabels: document.getElementById('modal-card-labels'),
    toggleLabelMenuBtn: document.getElementById('toggle-label-menu-btn'),
    labelMenuDropdown: document.getElementById('label-menu-dropdown'),
    modalCardAssignees: document.getElementById('modal-card-assignees'),
    toggleAssigneeMenuBtn: document.getElementById('toggle-assignee-menu-btn'),
    assigneeMenuDropdown: document.getElementById('assignee-menu-dropdown'),
    deleteCardBtn: document.getElementById('delete-card-btn'),

    // Copy Board
    copyBoardBtn: document.getElementById('copy-board-btn'),
    copyBoardModal: document.getElementById('copy-board-modal'),
    copyBoardForm: document.getElementById('copy-board-form'),
    copyBoardName: document.getElementById('copy-board-name'),
    copyBoardIncludeMembers: document.getElementById('copy-board-include-members')
  };

  // --- API CALLS (AJAX) ---
  const api = {
    async request(url, method = 'GET', data = null) {
      const options = { method, headers: {} };
      if (data) {
        options.headers['Content-Type'] = 'application/json';
        options.body = JSON.stringify(data);
      }
      const res = await fetch(url, options);
      if (res.status === 401) {
        showAuth();
        throw new Error('Unauthorized or session expired');
      }
      if (!res.ok) {
        let errText = res.statusText;
        try {
          const errJSON = await res.json();
          errText = errJSON.error || errText;
        } catch (e) {}
        throw new Error(errText);
      }
      if (res.status === 204) return null;
      return res.json();
    },

    // Auth
    register: (username, email, password) => api.request('/api/auth/register', 'POST', { username, email, password }),
    login: (username_or_email, password) => api.request('/api/auth/login', 'POST', { username_or_email, password }),
    logout: () => api.request('/api/auth/logout', 'POST'),
    me: () => api.request('/api/auth/me'),

    // Boards
    getBoards: () => api.request('/api/boards'),
    createBoard: (name, description) => api.request('/api/boards', 'POST', { name, description }),
    getBoard: (id) => api.request(`/api/boards/${id}`),
    updateBoard: (id, data) => api.request(`/api/boards/${id}`, 'PATCH', data),
    deleteBoard: (id) => api.request(`/api/boards/${id}`, 'DELETE'),
    inviteMember: (boardId, usernameOrEmail) => api.request(`/api/boards/${boardId}/members`, 'POST', { username_or_email: usernameOrEmail }),
    removeMember: (boardId, userId) => api.request(`/api/boards/${boardId}/members/${userId}`, 'DELETE'),
    getCollaborators: (boardId) => api.request(`/api/boards/${boardId}/collaborators`, 'GET'),
    copyBoard: (boardId, name, includeMembers) => api.request(`/api/boards/${boardId}/copy`, 'POST', { name, include_members: includeMembers }),

    // Lists
    createList: (boardId, name, position) => api.request(`/api/boards/${boardId}/lists`, 'POST', { name, position }),
    updateList: (id, name, position) => api.request(`/api/lists/${id}`, 'PATCH', { name, position }),
    deleteList: (id) => api.request(`/api/lists/${id}`, 'DELETE'),

    // Cards (Tasks)
    createCard: (listId, title, position) => api.request(`/api/lists/${listId}/tasks`, 'POST', { title, position }),
    getCard: (id) => api.request(`/api/tasks/${id}`),
    updateCard: (id, fields) => api.request(`/api/tasks/${id}`, 'PATCH', fields),
    deleteCard: (id) => api.request(`/api/tasks/${id}`, 'DELETE'),

    // Comments
    postComment: (cardId, content) => api.request(`/api/tasks/${cardId}/comments`, 'POST', { content }),
    getComments: (cardId) => api.request(`/api/tasks/${cardId}/comments`),

    // Checklist
    addChecklistItem: (cardId, title, position) => api.request(`/api/tasks/${cardId}/checklist`, 'POST', { title, position }),
    updateChecklistItem: (id, title, isCompleted) => api.request(`/api/checklist/${id}`, 'PATCH', { title, is_completed: isCompleted }),
    deleteChecklistItem: (id) => api.request(`/api/checklist/${id}`, 'DELETE'),

    // Assignees & Labels
    assignUser: (cardId, userId) => api.request(`/api/tasks/${cardId}/assignees`, 'POST', { user_id: userId }),
    unassignUser: (cardId, userId) => api.request(`/api/tasks/${cardId}/assignees`, 'DELETE', { user_id: userId }),
    getLabels: (boardId) => api.request(`/api/boards/${boardId}/labels`),
    addLabel: (cardId, labelId) => api.request(`/api/tasks/${cardId}/labels`, 'POST', { label_id: labelId }),
    removeLabel: (cardId, labelId) => api.request(`/api/tasks/${cardId}/labels`, 'DELETE', { label_id: labelId }),

    // Activities
    getActivities: (boardId) => api.request(`/api/boards/${boardId}/activities`)
  };

  // --- INITIALIZE APPLICATION ---
  async function initApp() {
    try {
      state.user = await api.me();
      showDashboard();
    } catch (err) {
      showAuth();
    }

    // Modal click-outside fallback setup
    setupModalLightDismissFallback(el.cardDetailModal);
    setupModalLightDismissFallback(el.createBoardModal);
    setupModalLightDismissFallback(el.inviteMemberModal);
    setupModalLightDismissFallback(el.boardSettingsModal);
    setupModalLightDismissFallback(el.copyBoardModal);
  }

  // Fallback for browsers that do not support <dialog closedby="any">
  function setupModalLightDismissFallback(dialog) {
    if (!dialog) return;
    if (!('closedBy' in HTMLDialogElement.prototype)) {
      dialog.addEventListener('click', (event) => {
        if (event.target !== dialog) return;
        const rect = dialog.getBoundingClientRect();
        const isDialogContent = (
          rect.top <= event.clientY &&
          event.clientY <= rect.top + rect.height &&
          rect.left <= event.clientX &&
          event.clientX <= rect.left + rect.width
        );
        if (isDialogContent) return;
        dialog.close();
      });
    }
  }

  // --- VIEW TRANSITIONS ---
  function showAuth() {
    // Reset state variables
    state.user = null;
    state.boards = [];
    state.activeBoard = null;
    state.activeCard = null;
    if (state.dnd) {
      state.dnd.destroy();
      state.dnd = null;
    }

    // Reset workspace UI
    el.boardsList.innerHTML = '';
    el.boardMembersList.innerHTML = '';
    el.listsContainer.innerHTML = '';
    el.boardTitle.textContent = 'Select a Board';
    el.boardDesc.textContent = 'Create or choose a board from the sidebar to manage your cards.';
    el.boardHeader.classList.add('hidden');
    el.inviteMemberBtn.classList.add('hidden');
    el.activityLogToggle.classList.add('hidden');
    el.boardSettingsBtn.classList.add('hidden');
    el.deleteBoardBtn.classList.add('hidden');
    el.copyBoardBtn.classList.add('hidden');

    // Reset theme to dark when no board selected
    applyTheme('dark');
    
    el.welcomeMessage.classList.remove('hidden');
    el.listsContainer.classList.add('hidden');
    el.activityPanel.classList.add('hidden');

    // Hide dashboard and show auth screen
    el.authContainer.classList.remove('hidden');
    el.dashboardContainer.classList.add('hidden');
  }

  async function showDashboard() {
    el.authContainer.classList.add('hidden');
    el.dashboardContainer.classList.remove('hidden');

    // Profile badge
    el.userName.textContent = state.user.username;
    el.userAvatar.textContent = state.user.username.substring(0, 2).toUpperCase();
    el.userAvatar.style.backgroundColor = state.user.avatar_color || '#6366f1';

    // Connect to user-level WebSocket for global events
    window.userWsClient.connect();

    // Load Boards list
    await loadBoards();
  }

  async function loadBoards() {
    try {
      state.boards = await api.getBoards() || [];
      renderBoardsList();
    } catch (err) {
      alert(`Error loading boards: ${err.message}`);
    }
  }

  function renderBoardsList() {
    el.boardsList.innerHTML = '';
    const showMyBoardsOnly = el.myBoardsFilter && el.myBoardsFilter.checked;
    const sortOption = el.boardsSort ? el.boardsSort.value : 'updated-desc';

    // Filter boards
    let filteredBoards = state.boards.filter(board => !showMyBoardsOnly || board.owner_id === state.user.id);

    // Sort boards
    filteredBoards.sort((a, b) => {
      switch (sortOption) {
        case 'updated-desc':
          return new Date(b.updated_at || b.created_at) - new Date(a.updated_at || a.created_at);
        case 'updated-asc':
          return new Date(a.updated_at || a.created_at) - new Date(b.updated_at || b.created_at);
        case 'alpha-asc':
          return a.name.localeCompare(b.name);
        case 'alpha-desc':
          return b.name.localeCompare(a.name);
        default:
          return 0;
      }
    });

    filteredBoards.forEach(board => {
      const li = document.createElement('li');
      const icon = board.icon || '📋';
      li.innerHTML = `
        <div class="board-link ${state.activeBoard && state.activeBoard.id === board.id ? 'active' : ''}" data-id="${board.id}">
          <span class="board-link-icon">${icon}</span>
          <span>${escapeHTML(board.name)}</span>
        </div>
      `;
      el.boardsList.appendChild(li);
    });
  }

  async function selectBoard(boardId) {
    try {
      const board = await api.getBoard(boardId);
      board.members = board.members || [];
      board.lists = board.lists || [];
      state.activeBoard = board;
      
      // Close activity drawer if open
      el.activityPanel.classList.add('hidden');

      // Update sidebar styling
      renderBoardsList();

      // Setup Header details
      el.boardTitle.textContent = board.name;
      el.boardDesc.textContent = board.description || 'No description provided.';
      el.boardHeader.classList.remove('hidden');
      el.inviteMemberBtn.classList.remove('hidden');
      el.activityLogToggle.classList.remove('hidden');
      el.boardSettingsBtn.classList.remove('hidden');
      el.copyBoardBtn.classList.remove('hidden');
      if (board.owner_id === state.user.id) {
        // User owns this board
        el.deleteBoardBtn.classList.remove('hidden');
        el.leaveBoardBtn.classList.add('hidden');
        el.boardOwner.classList.add('hidden');
      } else {
        // User doesn't own this board - show owner and leave button
        el.deleteBoardBtn.classList.add('hidden');
        el.leaveBoardBtn.classList.remove('hidden');
        // Find owner in members list
        const owner = board.members.find(m => m.id === board.owner_id);
        if (owner) {
          el.boardOwner.textContent = `Owner: ${owner.username}`;
          el.boardOwner.classList.remove('hidden');
        } else {
          el.boardOwner.classList.add('hidden');
        }
      }

      // Apply board theme
      applyTheme(board.theme || 'dark');

      el.welcomeMessage.classList.add('hidden');
      el.listsContainer.classList.remove('hidden');

      // Render avatars in header
      renderBoardMembersAvatars();

      // Render Board columns
      renderBoardLists();

      // Connect to Real-time WebSocket channel
      window.wsClient.connect(boardId);

      // Initialize Pointer Drag-and-Drop
      if (state.dnd) state.dnd.destroy();
      state.dnd = new KanbanDragAndDrop({
        onCardDropped: handleCardDropped
      });
      state.dnd.init();

    } catch (err) {
      alert(`Failed to load board: ${err.message}`);
    }
  }

  function renderBoardMembersAvatars() {
    el.boardMembersList.innerHTML = '';
    state.activeBoard.members.forEach(member => {
      const avatar = document.createElement('div');
      avatar.className = 'avatar-circle';
      avatar.textContent = member.username.substring(0, 2).toUpperCase();
      avatar.style.backgroundColor = member.avatar_color;
      avatar.title = `${member.username} (${member.email})`;
      el.boardMembersList.appendChild(avatar);
    });
  }

  function renderBoardLists() {
    el.listsContainer.innerHTML = '';

    state.activeBoard.lists.forEach(list => {
      const listCol = document.createElement('div');
      listCol.className = 'list-column glass-panel';
      listCol.dataset.id = list.id;

      listCol.innerHTML = `
        <div class="list-header" data-id="${list.id}">
          <div class="list-title-wrap">
            <input type="text" class="list-title-input" value="${escapeHTML(list.name)}">
            <span class="card-count" id="count-${list.id}">${list.tasks ? list.tasks.length : 0}</span>
          </div>
          <button class="btn-icon-sm delete-list-btn" title="Delete List">✖</button>
        </div>
        <div class="cards-container" data-list-id="${list.id}">
          <!-- Cards -->
        </div>
        <div class="add-card-wrap">
          <button class="add-card-btn-trigger">+ Add a card</button>
          <div class="card-composer hidden">
            <input type="text" placeholder="Enter a title for this card..." class="composer-title-input">
            <div class="composer-actions">
              <button class="btn btn-primary btn-sm save-new-card">Add Card</button>
              <button class="btn-icon-sm cancel-new-card">✖</button>
            </div>
          </div>
        </div>
      `;

      const cardsContainer = listCol.querySelector('.cards-container');
      
      // Render cards
      if (list.tasks) {
        list.tasks.forEach(task => {
          const card = createCardDOM(task);
          cardsContainer.appendChild(card);
        });
      }

      el.listsContainer.appendChild(listCol);
    });

    // Append "Add List" block
    const addListCol = document.createElement('div');
    addListCol.className = 'add-list-column';
    addListCol.innerHTML = `
      <button class="add-list-trigger">+ Add another list</button>
      <div class="list-composer hidden">
        <input type="text" placeholder="Enter list title..." class="list-composer-input">
        <div class="composer-actions">
          <button class="btn btn-primary btn-sm save-new-list">Add List</button>
          <button class="btn-icon-sm cancel-new-list">✖</button>
        </div>
      </div>
    `;
    el.listsContainer.appendChild(addListCol);
  }

  function createCardDOM(task) {
    const card = document.createElement('div');
    card.className = 'card-item';
    card.dataset.id = task.id;
    card.dataset.listId = task.list_id;

    // Build labels html
    let labelsHTML = '';
    if (task.labels && task.labels.length > 0) {
      labelsHTML = '<div class="card-labels">';
      task.labels.forEach(label => {
        labelsHTML += `<span class="label-pill" style="background-color: ${label.color}">${escapeHTML(label.name)}</span>`;
      });
      labelsHTML += '</div>';
    }

    // Build indicators html
    let indicatorsHTML = '';
    let hasIndicators = false;

    if (task.due_date) {
      hasIndicators = true;
      const dueDate = new Date(task.due_date);
      const isOverdue = dueDate < new Date() && !isToday(dueDate);
      const isDueToday = isToday(dueDate);
      
      let classVal = '';
      if (isOverdue) classVal = 'overdue';
      else if (isDueToday) classVal = 'due-soon';

      indicatorsHTML += `
        <span class="indicator ${classVal}" title="Due Date">
          📅 ${formatShortDate(dueDate)}
        </span>
      `;
    }

    // Check checklist progress
    if (task.checklist && task.checklist.length > 0) {
      hasIndicators = true;
      const completed = task.checklist.filter(item => item.is_completed).length;
      const total = task.checklist.length;
      const isDone = completed === total;
      indicatorsHTML += `
        <span class="indicator" style="${isDone ? 'color: var(--accent-success)' : ''}" title="Checklist progress">
          ☑️ ${completed}/${total}
        </span>
      `;
    }

    // Build Assignees HTML
    let assigneesHTML = '';
    if (task.assignees && task.assignees.length > 0) {
      assigneesHTML = '<div class="card-assignees">';
      task.assignees.forEach(assignee => {
        assigneesHTML += `
          <div class="avatar-circle" style="background-color: ${assignee.avatar_color}" title="${assignee.username}">
            ${assignee.username.substring(0, 2).toUpperCase()}
          </div>
        `;
      });
      assigneesHTML += '</div>';
    }

    card.innerHTML = `
      ${labelsHTML}
      <div class="card-title">${escapeHTML(task.title)}</div>
      <div class="card-footer">
        <div class="card-indicators">
          ${indicatorsHTML}
        </div>
        ${assigneesHTML}
      </div>
    `;

    return card;
  }

  // --- ACTION HANDLERS ---

  // Auth toggle
  el.showSignup.addEventListener('click', (e) => {
    e.preventDefault();
    el.loginForm.classList.add('hidden');
    el.registerForm.classList.remove('hidden');
    document.getElementById('auth-subtitle').textContent = 'Join team spaces instantly';
  });

  el.showLogin.addEventListener('click', (e) => {
    e.preventDefault();
    el.registerForm.classList.add('hidden');
    el.loginForm.classList.remove('hidden');
    document.getElementById('auth-subtitle').textContent = 'Elevate your team productivity';
  });

  // Auth Submit
  el.registerForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      const username = el.registerUsername.value;
      const email = el.registerEmail.value;
      const password = el.registerPassword.value;
      
      await api.register(username, email, password);
      // Auto login
      state.user = await api.login(username, password);
      
      // Clear inputs
      el.registerUsername.value = '';
      el.registerEmail.value = '';
      el.registerPassword.value = '';

      showDashboard();
    } catch (err) {
      alert(`Registration Failed: ${err.message}`);
    }
  });

  el.loginForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      const identifier = el.loginUsername.value;
      const password = el.loginPassword.value;

      state.user = await api.login(identifier, password);
      
      // Clear inputs
      el.loginUsername.value = '';
      el.loginPassword.value = '';

      showDashboard();
    } catch (err) {
      alert(`Login Failed: ${err.message}`);
    }
  });

  el.logoutBtn.addEventListener('click', async () => {
    try {
      await api.logout();
      window.wsClient.disconnect();
      window.userWsClient.disconnect();
      showAuth();
    } catch (err) {
      alert(`Logout Failed: ${err.message}`);
    }
  });

  // Sidebar expand / collapse
  el.sidebarToggle.addEventListener('click', () => {
    el.sidebar.classList.toggle('collapsed');
    if (el.sidebar.classList.contains('collapsed')) {
      el.sidebarToggle.textContent = '▶';
    } else {
      el.sidebarToggle.textContent = '◀';
    }
  });

  // My Boards filter
  el.myBoardsFilter.addEventListener('change', () => {
    renderBoardsList();
  });

  // Boards sort selector
  el.boardsSort.addEventListener('change', () => {
    renderBoardsList();
  });

  // Select board delegation
  el.boardsList.addEventListener('click', (e) => {
    const link = e.target.closest('.board-link');
    if (!link) return;
    const boardId = link.dataset.id;
    selectBoard(boardId);
  });

  // Welcome page Create Board click
  el.welcomeCreateBoardBtn.addEventListener('click', () => el.createBoardModal.showModal());
  el.createBoardBtn.addEventListener('click', () => el.createBoardModal.showModal());

  // Create Board Submit
  el.createBoardForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      const name = el.newBoardName.value;
      const desc = el.newBoardDesc.value;
      
      const newBoard = await api.createBoard(name, desc);
      
      el.newBoardName.value = '';
      el.newBoardDesc.value = '';
      el.createBoardModal.close();

      await loadBoards();
      selectBoard(newBoard.id);
    } catch (err) {
      alert(`Failed to create board: ${err.message}`);
    }
  });

  // Invite member open - fetch collaborators and show modal
  el.inviteMemberBtn.addEventListener('click', async () => {
    // Clear previous state
    el.inviteUsernameInput.value = '';
    el.collaboratorsList.innerHTML = '';
    el.collaboratorsSection.classList.add('hidden');

    // Fetch collaborators
    try {
      const collaborators = await api.getCollaborators(state.activeBoard.id);
      if (collaborators && collaborators.length > 0) {
        // Sort alphabetically by username
        collaborators.sort((a, b) => a.username.localeCompare(b.username));

        collaborators.forEach(user => {
          const item = document.createElement('div');
          item.className = 'collaborator-item';
          item.innerHTML = `
            <input type="checkbox" id="collab-${user.id}" value="${user.username}" data-user-id="${user.id}">
            <label for="collab-${user.id}">${escapeHTML(user.username)}</label>
          `;
          el.collaboratorsList.appendChild(item);
        });
        el.collaboratorsSection.classList.remove('hidden');
      }
    } catch (err) {
      // Silently fail - just don't show collaborators
      console.error('Failed to fetch collaborators:', err);
    }

    el.inviteMemberModal.showModal();
  });

  // Invite member submit - handle both checkboxes and text input
  el.inviteMemberForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    // Gather selected collaborators
    const selectedCheckboxes = el.collaboratorsList.querySelectorAll('input[type="checkbox"]:checked');
    const usersToInvite = Array.from(selectedCheckboxes).map(cb => cb.value);

    // Also include manual input if provided
    const manualInput = el.inviteUsernameInput.value.trim();
    if (manualInput) {
      usersToInvite.push(manualInput);
    }

    if (usersToInvite.length === 0) {
      alert('Please select at least one collaborator or enter a username/email.');
      return;
    }

    // Invite each user
    const errors = [];
    for (const username of usersToInvite) {
      try {
        const invitedUser = await api.inviteMember(state.activeBoard.id, username);
        // Update state local member list (prevent duplicates)
        if (!state.activeBoard.members.some(m => m.id === invitedUser.id)) {
          state.activeBoard.members.push(invitedUser);
        }
      } catch (err) {
        errors.push(`${username}: ${err.message}`);
      }
    }

    // Clear and close
    el.inviteUsernameInput.value = '';
    el.inviteMemberModal.close();
    renderBoardMembersAvatars();

    if (errors.length > 0) {
      alert(`Some invites failed:\n${errors.join('\n')}`);
    }
  });

  // Board Settings - open modal
  el.boardSettingsBtn.addEventListener('click', () => {
    el.settingsBoardName.value = state.activeBoard.name;
    el.settingsBoardDesc.value = state.activeBoard.description || '';
    el.settingsBoardIcon.value = state.activeBoard.icon || '📋';

    // Highlight current theme
    const currentTheme = state.activeBoard.theme || 'dark';
    el.themePicker.forEach(btn => {
      btn.classList.toggle('active', btn.dataset.theme === currentTheme);
    });

    // Populate members list
    renderSettingsMembersList();

    el.boardSettingsModal.showModal();
  });

  // Icon preset clicks
  el.iconPresets.forEach(btn => {
    btn.addEventListener('click', () => {
      el.settingsBoardIcon.value = btn.dataset.icon;
    });
  });

  function renderSettingsMembersList() {
    el.settingsMembersList.innerHTML = '';
    const isOwner = state.activeBoard.owner_id === state.user.id;

    state.activeBoard.members.forEach(member => {
      const isMemberOwner = member.id === state.activeBoard.owner_id;
      const li = document.createElement('li');
      li.className = 'settings-member-item';
      li.innerHTML = `
        <div class="settings-member-info">
          <div class="avatar-circle" style="background-color: ${member.avatar_color}">${escapeHTML(member.username.substring(0, 2).toUpperCase())}</div>
          <span class="settings-member-name">${escapeHTML(member.username)}</span>
          ${isMemberOwner ? '<span class="settings-member-role">Owner</span>' : ''}
        </div>
        ${!isMemberOwner && isOwner ? `<button type="button" class="btn-icon-sm remove-member-btn" data-user-id="${member.id}" title="Remove member">✖</button>` : ''}
      `;
      el.settingsMembersList.appendChild(li);
    });
  }

  // Handle remove member clicks
  el.settingsMembersList.addEventListener('click', async (e) => {
    const btn = e.target.closest('.remove-member-btn');
    if (!btn) return;

    const userId = btn.dataset.userId;
    const member = state.activeBoard.members.find(m => m.id === userId);
    if (!member) return;

    if (!confirm(`Remove ${member.username} from this board?`)) return;

    try {
      await api.removeMember(state.activeBoard.id, userId);
      state.activeBoard.members = state.activeBoard.members.filter(m => m.id !== userId);
      renderSettingsMembersList();
      renderBoardMembersAvatars();
    } catch (err) {
      alert(`Failed to remove member: ${err.message}`);
    }
  });

  // Theme picker clicks
  el.themePicker.forEach(btn => {
    btn.addEventListener('click', () => {
      el.themePicker.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
    });
  });

  // Board Settings form submit
  el.boardSettingsForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      const selectedTheme = document.querySelector('.theme-option.active')?.dataset.theme || 'dark';
      const selectedIcon = el.settingsBoardIcon.value || '📋';
      const updatedBoard = await api.updateBoard(state.activeBoard.id, {
        name: el.settingsBoardName.value,
        description: el.settingsBoardDesc.value,
        theme: selectedTheme,
        icon: selectedIcon
      });

      // Update local state
      state.activeBoard.name = updatedBoard.name;
      state.activeBoard.description = updatedBoard.description;
      state.activeBoard.theme = updatedBoard.theme;
      state.activeBoard.icon = updatedBoard.icon;

      // Update UI
      el.boardTitle.textContent = updatedBoard.name;
      el.boardDesc.textContent = updatedBoard.description || 'No description provided.';
      applyTheme(updatedBoard.theme || 'dark');

      // Also update the board in state.boards array
      const boardIdx = state.boards.findIndex(b => b.id === updatedBoard.id);
      if (boardIdx !== -1) {
        state.boards[boardIdx].name = updatedBoard.name;
        state.boards[boardIdx].description = updatedBoard.description;
        state.boards[boardIdx].theme = updatedBoard.theme;
        state.boards[boardIdx].icon = updatedBoard.icon;
        state.boards[boardIdx].updated_at = updatedBoard.updated_at;
      }

      // Update sidebar
      renderBoardsList();

      el.boardSettingsModal.close();
    } catch (err) {
      alert(`Failed to update board settings: ${err.message}`);
    }
  });

  // Delete Board click
  el.deleteBoardBtn.addEventListener('click', async () => {
    if (!confirm('Are you absolutely sure you want to delete this board? This will delete all columns, tasks, and history.')) {
      return;
    }
    try {
      await api.deleteBoard(state.activeBoard.id);
      window.wsClient.disconnect();
      state.activeBoard = null;

      el.boardHeader.classList.add('hidden');
      el.listsContainer.classList.add('hidden');
      el.welcomeMessage.classList.remove('hidden');

      await loadBoards();
    } catch (err) {
      alert(`Failed to delete board: ${err.message}`);
    }
  });

  // Copy Board - open modal
  el.copyBoardBtn.addEventListener('click', () => {
    if (!state.activeBoard) return;
    el.copyBoardName.value = `${state.activeBoard.name} copy`;
    el.copyBoardIncludeMembers.checked = false;
    el.copyBoardModal.showModal();
  });

  // Copy Board - form submit
  el.copyBoardForm.addEventListener('submit', async (e) => {
    e.preventDefault();
    try {
      const name = el.copyBoardName.value.trim();
      const includeMembers = el.copyBoardIncludeMembers.checked;

      if (!name) {
        alert('Please enter a name for the new board.');
        return;
      }

      const newBoard = await api.copyBoard(state.activeBoard.id, name, includeMembers);

      el.copyBoardModal.close();

      // Add the new board to the list and select it
      state.boards.push(newBoard);
      renderBoardsList();
      selectBoard(newBoard.id);
    } catch (err) {
      alert(`Failed to copy board: ${err.message}`);
    }
  });

  // Leave board (for non-owners)
  el.leaveBoardBtn.addEventListener('click', async () => {
    if (!confirm('Are you sure you want to leave this board? You will lose access unless invited again.')) {
      return;
    }
    try {
      // Save board ID and clean up state BEFORE API call to avoid WebSocket race conditions
      const boardId = state.activeBoard.id;
      state.boards = state.boards.filter(b => b.id !== boardId);
      state.activeBoard = null;

      // Disconnect WebSocket before API call so we don't receive the removal events
      window.wsClient.disconnect();

      // Reset UI immediately
      el.boardHeader.classList.add('hidden');
      el.listsContainer.classList.add('hidden');
      el.welcomeMessage.classList.remove('hidden');
      el.boardOwner.classList.add('hidden');
      el.leaveBoardBtn.classList.add('hidden');
      renderBoardsList();

      // Now call the API
      await api.removeMember(boardId, state.user.id);
    } catch (err) {
      alert(`Failed to leave board: ${err.message}`);
    }
  });

  // Horizontal Canvas Clicks & Input Delegation (Lists & Cards)
  el.listsContainer.addEventListener('click', async (e) => {
    // 1. Toggle Card Composer
    if (e.target.classList.contains('add-card-btn-trigger')) {
      const wrap = e.target.closest('.add-card-wrap');
      e.target.classList.add('hidden');
      wrap.querySelector('.card-composer').classList.remove('hidden');
      wrap.querySelector('.composer-title-input').focus();
    }

    // 2. Cancel Card Composer
    if (e.target.classList.contains('cancel-new-card')) {
      const wrap = e.target.closest('.add-card-wrap');
      wrap.querySelector('.card-composer').classList.add('hidden');
      wrap.querySelector('.add-card-btn-trigger').classList.remove('hidden');
      wrap.querySelector('.composer-title-input').value = '';
    }

    // 3. Save New Card
    if (e.target.classList.contains('save-new-card')) {
      const wrap = e.target.closest('.add-card-wrap');
      const listCol = e.target.closest('.list-column');
      const listId = listCol.dataset.id;
      const input = wrap.querySelector('.composer-title-input');
      const title = input.value.trim();

      if (!title) return;

      try {
        const container = listCol.querySelector('.cards-container');
        const position = container.children.length;
        
        await api.createCard(listId, title, position);
        
        // Reset composer
        wrap.querySelector('.card-composer').classList.add('hidden');
        wrap.querySelector('.add-card-btn-trigger').classList.remove('hidden');
        input.value = '';
      } catch (err) {
        alert(`Failed to create card: ${err.message}`);
      }
    }

    // 4. Toggle List Composer
    if (e.target.classList.contains('add-list-trigger')) {
      const wrap = e.target.closest('.add-list-column');
      e.target.classList.add('hidden');
      wrap.querySelector('.list-composer').classList.remove('hidden');
      wrap.querySelector('.list-composer-input').focus();
    }

    // 5. Cancel List Composer
    if (e.target.classList.contains('cancel-new-list')) {
      const wrap = e.target.closest('.add-list-column');
      wrap.querySelector('.list-composer').classList.add('hidden');
      wrap.querySelector('.add-list-trigger').classList.remove('hidden');
      wrap.querySelector('.list-composer-input').value = '';
    }

    // 6. Save New List
    if (e.target.classList.contains('save-new-list')) {
      const wrap = e.target.closest('.add-list-column');
      const input = wrap.querySelector('.list-composer-input');
      const name = input.value.trim();

      if (!name) return;

      try {
        const position = state.activeBoard.lists.length;
        await api.createList(state.activeBoard.id, name, position);
        
        input.value = '';
        wrap.querySelector('.list-composer').classList.add('hidden');
        wrap.querySelector('.add-list-trigger').classList.remove('hidden');
      } catch (err) {
        alert(`Failed to create list: ${err.message}`);
      }
    }

    // 7. Delete List Column
    if (e.target.classList.contains('delete-list-btn')) {
      const listCol = e.target.closest('.list-column');
      const listId = listCol.dataset.id;
      const listName = listCol.querySelector('.list-title-input').value;

      if (!confirm(`Are you sure you want to delete column "${listName}"? This will delete all cards inside.`)) {
        return;
      }

      try {
        await api.deleteList(listId);
      } catch (err) {
        alert(`Failed to delete list: ${err.message}`);
      }
    }

    // 8. Open Card Details Modal
    const cardItem = e.target.closest('.card-item');
    if (cardItem && !cardItem.classList.contains('card-ghost') && !e.target.classList.contains('avatar-circle')) {
      const cardId = cardItem.dataset.id;
      openCardDetailsModal(cardId);
    }
  });

  // Rename list on input blur or Enter key
  el.listsContainer.addEventListener('keydown', async (e) => {
    if (e.target.classList.contains('list-title-input') && e.key === 'Enter') {
      e.target.blur();
    }
  });

  el.listsContainer.addEventListener('focusout', async (e) => {
    if (e.target.classList.contains('list-title-input')) {
      const listCol = e.target.closest('.list-column');
      const listId = listCol.dataset.id;
      const newName = e.target.value.trim();

      const listIdx = state.activeBoard.lists.findIndex(l => l.id === listId);
      const currentList = state.activeBoard.lists[listIdx];

      if (!newName || newName === currentList.name) {
        e.target.value = currentList.name;
        return;
      }

      try {
        await api.updateList(listId, newName, currentList.position);
      } catch (err) {
        alert(`Failed to rename list: ${err.message}`);
        e.target.value = currentList.name;
      }
    }
  });

  // --- CARD DETAIL MODAL FUNCTIONALITY ---
  async function openCardDetailsModal(cardId) {
    try {
      const task = await api.getCard(cardId);
      state.activeCard = task;

      // Populate basic info
      el.modalCardTitle.value = task.title;
      el.modalCardDuedate.value = task.due_date ? task.due_date.substring(0, 10) : '';
      el.modalCardDesc.value = task.description;

      const listEl = state.activeBoard.lists.find(l => l.id === task.list_id);
      el.modalListName.textContent = listEl ? listEl.name : 'Unknown';

      // Load comments
      await refreshCommentsList(cardId);

      // Render Checklists
      renderChecklist();

      // Render assignees and labels
      renderModalLabelsAndAssignees();

      el.cardDetailModal.showModal();
    } catch (err) {
      alert(`Failed to load card details: ${err.message}`);
    }
  }

  function renderModalLabelsAndAssignees() {
    // Labels List inline
    el.modalCardLabels.innerHTML = '';
    if (state.activeCard.labels) {
      state.activeCard.labels.forEach(label => {
        const pill = document.createElement('span');
        pill.className = 'label-pill';
        pill.style.backgroundColor = label.color;
        pill.textContent = label.name;
        pill.title = 'Click to remove';
        pill.addEventListener('click', () => toggleLabel(label.id));
        el.modalCardLabels.appendChild(pill);
      });
    }

    // Assignees inline
    el.modalCardAssignees.innerHTML = '';
    if (state.activeCard.assignees) {
      state.activeCard.assignees.forEach(assignee => {
        const circle = document.createElement('div');
        circle.className = 'avatar-circle';
        circle.style.backgroundColor = assignee.avatar_color;
        circle.textContent = assignee.username.substring(0, 2).toUpperCase();
        circle.title = `${assignee.username} (Click to remove)`;
        circle.addEventListener('click', () => toggleAssignee(assignee.id));
        el.modalCardAssignees.appendChild(circle);
      });
    }

    // Build Assignee Dropdown menu items
    el.assigneeMenuDropdown.innerHTML = '';
    state.activeBoard.members.forEach(member => {
      const isAssigned = state.activeCard.assignees && state.activeCard.assignees.some(a => a.id === member.id);
      const label = document.createElement('label');
      label.className = 'dropdown-item-label';
      label.innerHTML = `
        <input type="checkbox" ${isAssigned ? 'checked' : ''}>
        <span>${escapeHTML(member.username)}</span>
      `;
      label.querySelector('input').addEventListener('change', () => toggleAssignee(member.id));
      el.assigneeMenuDropdown.appendChild(label);
    });

    // Build Labels Dropdown menu items
    // First fetch labels of this board from state or local request
    el.labelMenuDropdown.innerHTML = '';
    api.getLabels(state.activeBoard.id).then(labels => {
      const safeLabels = labels || [];
      safeLabels.forEach(label => {
        const isLabelled = state.activeCard.labels && state.activeCard.labels.some(l => l.id === label.id);
        const lbl = document.createElement('label');
        lbl.className = 'dropdown-item-label';
        lbl.innerHTML = `
          <input type="checkbox" ${isLabelled ? 'checked' : ''}>
          <span class="label-pill" style="background-color: ${label.color}">${escapeHTML(label.name)}</span>
        `;
        lbl.querySelector('input').addEventListener('change', () => toggleLabel(label.id));
        el.labelMenuDropdown.appendChild(lbl);
      });
    });
  }

  // Toggle Dropdown visibility
  el.toggleAssigneeMenuBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    el.assigneeMenuDropdown.classList.toggle('hidden');
    el.labelMenuDropdown.classList.add('hidden');
  });

  el.toggleLabelMenuBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    el.labelMenuDropdown.classList.toggle('hidden');
    el.assigneeMenuDropdown.classList.add('hidden');
  });

  document.addEventListener('click', () => {
    el.labelMenuDropdown.classList.add('hidden');
    el.assigneeMenuDropdown.classList.add('hidden');
  });

  el.labelMenuDropdown.addEventListener('click', (e) => e.stopPropagation());
  el.assigneeMenuDropdown.addEventListener('click', (e) => e.stopPropagation());

  async function toggleAssignee(memberId) {
    const isAssigned = state.activeCard.assignees && state.activeCard.assignees.some(a => a.id === memberId);
    try {
      if (isAssigned) {
        await api.unassignUser(state.activeCard.id, memberId);
      } else {
        await api.assignUser(state.activeCard.id, memberId);
      }
      // Re-fetch card details
      const updatedCard = await api.getCard(state.activeCard.id);
      state.activeCard = updatedCard;
      renderModalLabelsAndAssignees();
    } catch (err) {
      alert(`Assignment failed: ${err.message}`);
    }
  }

  async function toggleLabel(labelId) {
    const isLabelled = state.activeCard.labels && state.activeCard.labels.some(l => l.id === labelId);
    try {
      if (isLabelled) {
        await api.removeLabel(state.activeCard.id, labelId);
      } else {
        await api.addLabel(state.activeCard.id, labelId);
      }
      // Re-fetch card details
      const updatedCard = await api.getCard(state.activeCard.id);
      state.activeCard = updatedCard;
      renderModalLabelsAndAssignees();
    } catch (err) {
      alert(`Label update failed: ${err.message}`);
    }
  }

  // Rename card from within modal
  el.modalCardTitle.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') e.target.blur();
  });

  el.modalCardTitle.addEventListener('focusout', async () => {
    const newTitle = el.modalCardTitle.value.trim();
    if (!newTitle || newTitle === state.activeCard.title) {
      el.modalCardTitle.value = state.activeCard.title;
      return;
    }

    try {
      await api.updateCard(state.activeCard.id, {
        title: newTitle,
        description: state.activeCard.description,
        list_id: state.activeCard.list_id,
        position: state.activeCard.position
      });
      state.activeCard.title = newTitle;
    } catch (err) {
      alert(`Failed to update card title: ${err.message}`);
      el.modalCardTitle.value = state.activeCard.title;
    }
  });

  // Save description
  el.saveDescBtn.addEventListener('click', async () => {
    const newDesc = el.modalCardDesc.value;
    try {
      await api.updateCard(state.activeCard.id, {
        title: state.activeCard.title,
        description: newDesc,
        list_id: state.activeCard.list_id,
        position: state.activeCard.position
      });
      state.activeCard.description = newDesc;
      alert('Description saved successfully!');
    } catch (err) {
      alert(`Failed to save description: ${err.message}`);
    }
  });

  // Save due date
  el.saveDuedateBtn.addEventListener('click', async () => {
    const dateVal = el.modalCardDuedate.value;
    let dueDateObj = null;
    if (dateVal) {
      dueDateObj = new Date(dateVal);
    }
    
    try {
      await api.updateCard(state.activeCard.id, {
        title: state.activeCard.title,
        description: state.activeCard.description,
        list_id: state.activeCard.list_id,
        position: state.activeCard.position,
        due_date: dueDateObj
      });
      if (dueDateObj) {
        state.activeCard.due_date = dueDateObj.toISOString();
      } else {
        state.activeCard.due_date = null;
      }
      alert('Due date updated!');
    } catch (err) {
      alert(`Failed to save date: ${err.message}`);
    }
  });

  // Delete Card
  el.deleteCardBtn.addEventListener('click', async () => {
    if (!confirm('Are you sure you want to delete this card?')) return;
    try {
      await api.deleteCard(state.activeCard.id);
      el.cardDetailModal.close();
    } catch (err) {
      alert(`Failed to delete card: ${err.message}`);
    }
  });

  // --- CHECKLIST CORE ---

  function renderChecklist() {
    el.checklistItemsList.innerHTML = '';
    const items = state.activeCard.checklist || [];
    
    if (items.length === 0) {
      el.checklistProgress.textContent = '0%';
      el.checklistProgressBar.style.width = '0%';
      return;
    }

    const completed = items.filter(item => item.is_completed).length;
    const total = items.length;
    const percent = Math.round((completed / total) * 100);

    el.checklistProgress.textContent = `${percent}%`;
    el.checklistProgressBar.style.width = `${percent}%`;

    items.forEach(item => {
      const itemRow = document.createElement('div');
      itemRow.className = `checklist-item ${item.is_completed ? 'completed' : ''}`;
      itemRow.innerHTML = `
        <div class="checklist-item-check">
          <input type="checkbox" ${item.is_completed ? 'checked' : ''}>
          <span>${escapeHTML(item.title)}</span>
        </div>
        <button class="btn-icon-sm delete-checklist-item-btn" data-id="${item.id}">✖</button>
      `;

      itemRow.querySelector('input').addEventListener('change', async (e) => {
        try {
          await api.updateChecklistItem(item.id, item.title, e.target.checked);
          item.is_completed = e.target.checked;
          if (e.target.checked) {
            itemRow.classList.add('completed');
          } else {
            itemRow.classList.remove('completed');
          }
          // Recalculate percent
          const updatedCard = await api.getCard(state.activeCard.id);
          state.activeCard = updatedCard;
          renderChecklist();
        } catch (err) {
          alert(`Checklist toggle failed: ${err.message}`);
          e.target.checked = !e.target.checked;
        }
      });

      itemRow.querySelector('.delete-checklist-item-btn').addEventListener('click', async () => {
        try {
          await api.deleteChecklistItem(item.id);
          state.activeCard.checklist = state.activeCard.checklist.filter(i => i.id !== item.id);
          renderChecklist();
        } catch (err) {
          alert(`Checklist item deletion failed: ${err.message}`);
        }
      });

      el.checklistItemsList.appendChild(itemRow);
    });
  }

  el.addChecklistBtn.addEventListener('click', async () => {
    const title = el.newChecklistTitle.value.trim();
    if (!title) return;

    try {
      const position = state.activeCard.checklist ? state.activeCard.checklist.length : 0;
      const item = await api.addChecklistItem(state.activeCard.id, title, position);
      
      if (!state.activeCard.checklist) state.activeCard.checklist = [];
      state.activeCard.checklist.push(item);
      el.newChecklistTitle.value = '';
      
      renderChecklist();
    } catch (err) {
      alert(`Failed to add checklist item: ${err.message}`);
    }
  });

  // --- COMMENTS CORE ---

  async function refreshCommentsList(cardId) {
    try {
      const comments = await api.getComments(cardId) || [];
      el.commentsList.innerHTML = '';
      
      comments.forEach(comment => {
        const commentDiv = document.createElement('div');
        commentDiv.className = 'comment-card';
        commentDiv.innerHTML = `
          <div class="avatar-circle" style="background-color: ${comment.avatar_color || '#6366f1'}">
            ${comment.username.substring(0, 2).toUpperCase()}
          </div>
          <div style="flex: 1">
            <div class="comment-header">
              <span class="comment-author">${escapeHTML(comment.username)}</span>
              <span class="comment-time">${formatTimeAgo(new Date(comment.created_at))}</span>
            </div>
            <div class="comment-body">${escapeHTML(comment.content)}</div>
          </div>
        `;
        el.commentsList.appendChild(commentDiv);
      });
    } catch (err) {
      console.error(`Failed to refresh comments: ${err.message}`);
    }
  }

  el.postCommentBtn.addEventListener('click', async () => {
    const content = el.newCommentContent.value.trim();
    if (!content) return;

    try {
      await api.postComment(state.activeCard.id, content);
      el.newCommentContent.value = '';
      await refreshCommentsList(state.activeCard.id);
    } catch (err) {
      alert(`Failed to post comment: ${err.message}`);
    }
  });

  // --- DRAG AND DROP HANDLER ---
  async function handleCardDropped(cardId, startListId, endListId, newPosition) {
    try {
      // Find the card task details
      const listIdx = state.activeBoard.lists.findIndex(l => l.id === startListId);
      const task = state.activeBoard.lists[listIdx].tasks.find(t => t.id === cardId);

      // Perform optimistic UI update locally
      // The websocket message from the server will keep other clients in sync.
      // We send the REST API call to save the new coordinates.
      await api.updateCard(cardId, {
        title: task.title,
        description: task.description,
        list_id: endListId,
        position: newPosition,
        due_date: task.due_date ? new Date(task.due_date) : null
      });

    } catch (err) {
      console.error('Failed to update card coordinates in database:', err);
      // Rollback to database representation
      alert(`Movement synchronization failed: ${err.message}. Refreshing board...`);
      selectBoard(state.activeBoard.id);
    }
  }

  // --- ACTIVITIES FEED DRAWER ---
  el.activityLogToggle.addEventListener('click', async () => {
    el.activityPanel.classList.toggle('hidden');
    if (!el.activityPanel.classList.contains('hidden')) {
      await refreshActivitiesFeed();
    }
  });

  el.activityPanelClose.addEventListener('click', () => el.activityPanel.classList.add('hidden'));

  async function refreshActivitiesFeed() {
    try {
      const activities = await api.getActivities(state.activeBoard.id) || [];
      el.activityList.innerHTML = '';
      activities.forEach(activity => {
        const li = document.createElement('li');
        li.className = 'activity-item';
        li.innerHTML = `
          <div>
            <span class="activity-actor">${escapeHTML(activity.username)}</span>
            <span class="activity-desc">${escapeHTML(activity.description)}</span>
          </div>
          <span class="activity-time">${formatTimeAgo(new Date(activity.created_at))}</span>
        `;
        el.activityList.appendChild(li);
      });
    } catch (err) {
      console.error('Failed to refresh activity log:', err);
    }
  }

  // --- REAL-TIME WEBSOCKET ROUTING ---
  document.addEventListener('trbillo-ws-message', async (event) => {
    const message = event.detail;

    // Handle user-level events first (they use 'type' not 'event')
    if (message.type) {
      switch (message.type) {
        case 'removed_from_board':
          state.boards = state.boards.filter(b => b.id !== message.board_id);
          if (state.activeBoard && state.activeBoard.id === message.board_id) {
            alert('You have been removed from this board.');
            window.wsClient.disconnect();
            state.activeBoard = null;
            el.boardHeader.classList.add('hidden');
            el.listsContainer.classList.add('hidden');
            el.welcomeMessage.classList.remove('hidden');
            el.boardSettingsBtn.classList.add('hidden');
            el.deleteBoardBtn.classList.add('hidden');
            el.leaveBoardBtn.classList.add('hidden');
            el.boardOwner.classList.add('hidden');
            el.copyBoardBtn.classList.add('hidden');
            applyTheme('dark');
          }
          renderBoardsList();
          return;

        case 'added_to_board':
          if (!state.boards) state.boards = [];
          if (message.board && !state.boards.some(b => b.id === message.board.id)) {
            state.boards.push(message.board);
            renderBoardsList();
          }
          return;
      }
    }

    // Board-level events require an active board that matches
    if (!state.activeBoard || message.board_id !== state.activeBoard.id) return;

    const eventName = message.event;
    const data = message.data;


    // Refresh board activities feed if drawer is open
    if (!el.activityPanel.classList.contains('hidden')) {
      refreshActivitiesFeed();
    }

    switch (eventName) {
      case 'list_created':
        // Append list to state and re-render board columns (prevent duplicates)
        if (!state.activeBoard.lists.some(l => l.id === data.id)) {
          state.activeBoard.lists.push(data);
          renderBoardLists();
        }
        break;

      case 'list_updated':
        // Update list properties
        const lIdx = state.activeBoard.lists.findIndex(l => l.id === data.id);
        if (lIdx !== -1) {
          state.activeBoard.lists[lIdx].name = data.name;
          state.activeBoard.lists[lIdx].position = data.position;
          
          // Re-render
          renderBoardLists();
        }
        break;

      case 'list_deleted':
        state.activeBoard.lists = state.activeBoard.lists.filter(l => l.id !== data.list_id);
        renderBoardLists();
        break;

      case 'card_created':
        const targetList = state.activeBoard.lists.find(l => l.id === data.list_id);
        if (targetList) {
          if (!targetList.tasks) targetList.tasks = [];

          // Check if card already exists (prevent duplicates)
          const cardExists = targetList.tasks.some(t => t.id === data.id);
          if (cardExists) break;

          // Also check if card is already in DOM
          const existingCardEl = document.querySelector(`.card-item[data-id="${data.id}"]`);
          if (existingCardEl) break;

          targetList.tasks.push(data);

          // Append in DOM dynamically for snappy visual
          const cardsCont = document.querySelector(`.cards-container[data-list-id="${data.list_id}"]`);
          if (cardsCont) {
            const cardEl = createCardDOM(data);
            cardsCont.appendChild(cardEl);
            updateCardCountHeader(data.list_id, targetList.tasks.length);
          }
        }
        break;

      case 'card_updated':
        // Move task or update task details
        const updatedTask = data.task;
        const oldListId = data.old_list_id;

        // Remove from old list state
        const oldL = state.activeBoard.lists.find(l => l.id === oldListId);
        if (oldL && oldL.tasks) {
          oldL.tasks = oldL.tasks.filter(t => t.id !== updatedTask.id);
          updateCardCountHeader(oldListId, oldL.tasks.length);
        }

        // Insert into new list state at position
        const newL = state.activeBoard.lists.find(l => l.id === updatedTask.list_id);
        if (newL) {
          if (!newL.tasks) newL.tasks = [];
          
          // Remove if duplicate exists
          newL.tasks = newL.tasks.filter(t => t.id !== updatedTask.id);
          
          // Insert at specified position
          newL.tasks.splice(updatedTask.position, 0, updatedTask);
          updateCardCountHeader(updatedTask.list_id, newL.tasks.length);
        }

        // Re-draw board lists to align coordinates correctly
        // We only redraw if the user is NOT actively dragging
        if (!state.dnd.activeDrag) {
          renderBoardLists();
        }

        // If details modal is open for this updated card, refresh modal fields
        if (state.activeCard && state.activeCard.id === updatedTask.id) {
          state.activeCard = updatedTask;
          // Refresh modal fields
          el.modalCardTitle.value = updatedTask.title;
          el.modalCardDesc.value = updatedTask.description;
          el.modalCardDuedate.value = updatedTask.due_date ? updatedTask.due_date.substring(0, 10) : '';
          const curList = state.activeBoard.lists.find(l => l.id === updatedTask.list_id);
          el.modalListName.textContent = curList ? curList.name : 'Unknown';
          renderChecklist();
          renderModalLabelsAndAssignees();
        }
        break;

      case 'card_deleted':
        const listToClean = state.activeBoard.lists.find(l => l.id === data.list_id);
        if (listToClean && listToClean.tasks) {
          listToClean.tasks = listToClean.tasks.filter(t => t.id !== data.task_id);
          updateCardCountHeader(data.list_id, listToClean.tasks.length);
        }

        // Remove card node from DOM
        const cardNode = document.querySelector(`.card-item[data-id="${data.task_id}"]`);
        if (cardNode) {
          cardNode.parentNode.removeChild(cardNode);
        }

        // Close details modal if open for deleted card
        if (state.activeCard && state.activeCard.id === data.task_id) {
          el.cardDetailModal.close();
          state.activeCard = null;
          alert('This card has been deleted by another collaborator.');
        }
        break;

      case 'comment_added':
        if (state.activeCard && state.activeCard.id === data.task_id) {
          await refreshCommentsList(data.task_id);
        }
        
        // Refresh checklist progress indicators in board cards
        // Refresh board cards details representation
        const cTask = findTaskByID(data.task_id);
        if (cTask) {
          // Re-render card in DOM
          const oldCardNode = document.querySelector(`.card-item[data-id="${data.task_id}"]`);
          if (oldCardNode) {
            const newCardNode = createCardDOM(cTask);
            oldCardNode.parentNode.replaceChild(newCardNode, oldCardNode);
          }
        }
        break;

      case 'card_checklist_updated':
        const chTaskId = data.task_id;
        const newChecklist = data.checklist;

        const chTask = findTaskByID(chTaskId);
        if (chTask) {
          chTask.checklist = newChecklist;
          
          // If details modal open
          if (state.activeCard && state.activeCard.id === chTaskId) {
            state.activeCard.checklist = newChecklist;
            renderChecklist();
          }

          // Update card item DOM representation
          const oldNode = document.querySelector(`.card-item[data-id="${chTaskId}"]`);
          if (oldNode) {
            const newNode = createCardDOM(chTask);
            oldNode.parentNode.replaceChild(newNode, oldNode);
          }
        }
        break;

      case 'card_assignees_updated':
      case 'card_labels_updated':
        // For assignees/labels update, fetch the card and re-render
        try {
          const freshTask = await api.getCard(data.task_id || data.task.id);
          const localTask = findTaskByID(freshTask.id);
          if (localTask) {
            localTask.assignees = freshTask.assignees;
            localTask.labels = freshTask.labels;
            localTask.checklist = freshTask.checklist;

            // Re-render card in list
            const oldCardNode = document.querySelector(`.card-item[data-id="${freshTask.id}"]`);
            if (oldCardNode) {
              const newCardNode = createCardDOM(localTask);
              oldCardNode.parentNode.replaceChild(newCardNode, oldCardNode);
            }

            if (state.activeCard && state.activeCard.id === freshTask.id) {
              state.activeCard = freshTask;
              renderModalLabelsAndAssignees();
            }
          }
        } catch (e) {
          console.error(e);
        }
        break;

      case 'board_updated':
        if (data.board && data.board.name) {
          state.activeBoard.name = data.board.name;
          state.activeBoard.description = data.board.description;
          state.activeBoard.theme = data.board.theme;
          state.activeBoard.icon = data.board.icon;
          el.boardTitle.textContent = data.board.name;
          el.boardDesc.textContent = data.board.description || 'No description provided.';
          applyTheme(data.board.theme || 'dark');

          // Also update state.boards array
          const boardIdx = state.boards.findIndex(b => b.id === data.board.id);
          if (boardIdx !== -1) {
            state.boards[boardIdx].name = data.board.name;
            state.boards[boardIdx].description = data.board.description;
            state.boards[boardIdx].theme = data.board.theme;
            state.boards[boardIdx].icon = data.board.icon;
            state.boards[boardIdx].updated_at = data.board.updated_at;
          }

          renderBoardsList();
        }
        break;

      case 'member_added':
        // Prevent duplicate members
        if (!state.activeBoard.members.some(m => m.id === data.user.id)) {
          state.activeBoard.members.push(data.user);
          renderBoardMembersAvatars();
        }
        break;

      case 'member_removed':
        state.activeBoard.members = state.activeBoard.members.filter(m => m.id !== data.user_id);
        renderBoardMembersAvatars();
        if (data.user_id === state.user.id) {
          // I was removed from this board
          alert('You have been removed from this board.');
          window.wsClient.disconnect();

          // Remove this board from state.boards
          state.boards = state.boards.filter(b => b.id !== state.activeBoard.id);
          state.activeBoard = null;

          // Reset UI
          el.boardHeader.classList.add('hidden');
          el.listsContainer.classList.add('hidden');
          el.welcomeMessage.classList.remove('hidden');
          el.boardSettingsBtn.classList.add('hidden');
          el.deleteBoardBtn.classList.add('hidden');
          el.leaveBoardBtn.classList.add('hidden');
          el.boardOwner.classList.add('hidden');
          el.copyBoardBtn.classList.add('hidden');
          applyTheme('dark');

          // Re-render boards list immediately
          renderBoardsList();
        }
        break;
    }
  });

  // Helper: Find task in state
  function findTaskByID(id) {
    if (!state.activeBoard) return null;
    for (let list of state.activeBoard.lists) {
      if (list.tasks) {
        const t = list.tasks.find(x => x.id === id);
        if (t) return t;
      }
    }
    return null;
  }

  function updateCardCountHeader(listId, count) {
    const counter = document.getElementById(`count-${listId}`);
    if (counter) {
      counter.textContent = count;
    }
  }

  // --- HELPERS ---
  function applyTheme(themeName) {
    // Remove existing theme or set to dark by default
    if (!themeName || themeName === 'dark') {
      document.body.removeAttribute('data-theme');
    } else {
      document.body.setAttribute('data-theme', themeName);
    }
  }

  function escapeHTML(str) {
    if (!str) return '';
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  function formatShortDate(date) {
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  }

  function isToday(date) {
    const today = new Date();
    return date.getDate() === today.getDate() &&
      date.getMonth() === today.getMonth() &&
      date.getFullYear() === today.getFullYear();
  }

  function formatTimeAgo(date) {
    const seconds = Math.floor((new Date() - date) / 1000);
    let interval = Math.floor(seconds / 31536000);
    
    if (interval >= 1) return interval + "y ago";
    interval = Math.floor(seconds / 2592000);
    if (interval >= 1) return interval + "mo ago";
    interval = Math.floor(seconds / 86400);
    if (interval >= 1) return interval + "d ago";
    interval = Math.floor(seconds / 3600);
    if (interval >= 1) return interval + "h ago";
    interval = Math.floor(seconds / 60);
    if (interval >= 1) return interval + "m ago";
    return "just now";
  }

  // Run Startup
  initApp();
});
