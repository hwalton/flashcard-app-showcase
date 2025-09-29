# Download Make. (Makefile)

# Install AIR (on restart):
```
export PATH=$HOME/go/bin:/usr/local/go/bin:$PATH
export PATH="$HOME/go/bin:$PATH"
source ~/.bashrc
```

# Development:
```
make watch-css
cd src/
air
```


# Build for production:

```
make build-css
docker build -f Dockerfile -t flashcards-app .
```

# Set up Supabase:
- Create bucket `flashcard-assets` in Supabase Storage.
- Create bucket policy on `flashcard-assets`:
    - Full customisation
    - Name: Allow authenticated read
    - Allowed operation: SELECT
    - Target roles: authenticated
    - Policy definition: bucket_id = 'flashcard-assets'
- Create bucket policy on `flashcard-assets`:
    - Full customisation
    - Name: Allow service_role to upload/replace files
    - Allowed operation: Select, Insert, Update
    - Target roles: service_role
    - Policy definition: bucket_id = 'flashcard-assets'
    - A service-role key will need to be created for utils_prod/.env
- Authentication:
    - URL configuration: set the site URL (with no trailing slash)
    - Set up email templates: (see supabase-auth-emails)

    - Set up SMTP mail domain
-Sessions
    - Set JWT access token expiry time to 604800 seconds (7 days)